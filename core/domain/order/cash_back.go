/**
 * Copyright 2015 @ z3q.net.
 * name : cash_back
 * author : jarryliu
 * date : -- :
 * description :
 * history :
 */
package order

import (
	"fmt"
	"go2o/core/domain/interface/member"
	"go2o/core/domain/interface/merchant"
	"go2o/core/domain/interface/order"
	"go2o/core/domain/interface/promotion"
	"go2o/core/infrastructure/domain"
	"go2o/core/infrastructure/format"
	"strconv"
	"strings"
	"time"
)

// 获取推荐数组
func (o *subOrderImpl) getReferArr(memberId int, level int) []int {
	arr := make([]int, level)
	i := 0
	referId := memberId
	for i <= level-1 {
		rl := o._memberRep.GetRelation(referId)
		if rl == nil || rl.RefereesId <= 0 {
			break
		}
		arr[i] = rl.RefereesId
		referId = arr[i]
		i++
	}
	return arr
}

func (o *subOrderImpl) handleCashBack() error {
	gobConf := o._valRep.GetGlobMchSaleConf()
	if !gobConf.FxSalesEnabled {
		return nil
	}

	v := o._value
	mch, err := o._mchRep.GetMerchant(v.VendorId)
	if err != nil {
		return err
	}

	buyer := o.GetBuyer()
	now := time.Now().Unix()

	//******* 返现到账户  ************
	var back_fee float32
	saleConf := mch.ConfManager().GetSaleConf()

	if saleConf.CashBackPercent > 0 {
		back_fee = v.FinalAmount * saleConf.CashBackPercent
		//将此次消费记入会员账户
		err = o.updateShoppingMemberBackFee(mch.GetValue().Name, buyer,
			back_fee*saleConf.CashBackMemberPercent, now)
		domain.HandleError(err, "domain")

	}

	// 处理返现促销
	//todo: ????
	//o.handleCashBackPromotions(mch, m)
	// 三级返现
	if back_fee > 0 {
		err = o.backFor3R(mch, buyer, back_fee, now)
	}
	return err
}

func (o *subOrderImpl) updateMemberAccount(m member.IMember,
	ptName, mName string, fee float32, unixTime int64) error {
	if fee > 0 {
		//更新账户
		acc := m.GetAccount()
		acv := acc.GetValue()
		acv.PresentBalance += fee
		acv.TotalPresentFee += fee
		acv.UpdateTime = unixTime
		_, err := acc.Save()
		if err == nil {
			//给自己返现
			tit := fmt.Sprintf("订单:%s(商户:%s,会员:%s)收入￥%.2f元",
				o._value.OrderNo, ptName, mName, fee)
			err = acc.PresentBalance(tit, o._value.OrderNo, fee)
		}
		return err
	}
	return nil
}

// 三级返现
func (o *subOrderImpl) backFor3R(mch merchant.IMerchant, m member.IMember,
	back_fee float32, unixTime int64) (err error) {
	if back_fee > 0 {
		i := 0
		mName := m.Profile().GetProfile().Name
		saleConf := mch.ConfManager().GetSaleConf()
		percent := saleConf.CashBackTg2Percent
		for i < 2 {
			rl := m.GetRelation()
			if rl == nil || rl.RefereesId == 0 {
				break
			}

			m = o._memberRep.GetMember(rl.RefereesId)
			if m == nil {
				break
			}

			if i == 1 {
				percent = saleConf.CashBackTg1Percent
			}

			err = o.updateMemberAccount(m, mch.GetValue().Name, mName,
				back_fee*percent, unixTime)
			if err != nil {
				domain.HandleError(err, "domain")
				break
			}
			i++
		}
	}
	return err
}

func HandleCashBackDataTag(m member.IMember, order *order.Order,
	c promotion.ICashBackPromotion, memberRep member.IMemberRep) {
	data := c.GetDataTag()
	var level int = 0
	for k, _ := range data {
		if strings.HasPrefix(k, "G") {
			if l, err := strconv.Atoi(k[1:]); err == nil && l > level {
				level = l
			}
		}
	}
	//log.Println("[ Back][ Level] - ",level)
	cashBack3R(level, m, order, c, memberRep)
}

func cashBack3R(level int, m member.IMember, order *order.Order, c promotion.ICashBackPromotion, memberRep member.IMemberRep) {

	dt := c.GetDataTag()

	var cm member.IMember = m
	var pm member.IMember = m

	// fmt.Println("------ START BACK ------")

	var backFunc = func(m member.IMember, parentM member.IMember, fee int) {
		// fmt.Println("---------[ back ]",parentM.GetValue().Name,fee)
		backCashForMember(m, order, fee, parentM.Profile().GetProfile().Name)
	}
	var i int = 0
	for true {
		rl := cm.GetRelation()
		// fmt.Println("-------- BACK - ID - ",rl.InvitationMemberId)
		if rl == nil || rl.RefereesId == 0 {
			break
		}

		cm = memberRep.GetMember(rl.RefereesId)

		// fmt.Println("-------- BACK ",cm == nil)
		if m == nil {
			break
		}

		if fee, err := strconv.Atoi(dt[fmt.Sprintf("G%d", i)]); err == nil {
			//log.Println("[ Back][ Cash] - ",i," back ",fee)
			backFunc(cm, pm, fee)
		}

		pm = cm

		i++
		if i >= level {
			break
		}
	}
}

func backCashForMember(m member.IMember, order *order.Order,
	fee int, refName string) error {
	//更新账户
	acc := m.GetAccount()
	acv := acc.GetValue()
	bFee := float32(fee)
	acv.PresentBalance += bFee // 更新赠送余额
	acv.TotalPresentFee += bFee
	acv.UpdateTime = time.Now().Unix()
	_, err := acc.Save()

	if err == nil {
		tit := fmt.Sprintf("推广返现￥%s元,订单号:%s,来源：%s",
			format.FormatFloat(bFee), order.OrderNo, refName)
		err = acc.PresentBalance(tit, order.OrderNo, float32(fee))
	}
	return err
}
