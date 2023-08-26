/*
 * @Author: aztec
 * @Date: 2022-03-30 13:14:11
 * @LastEditors: aztec
 * @LastEditTime: 2023-03-02 16:37:41
 * @FilePath: \stratergyc:\work\svn\go\src\dagger\cex\okexv5\future_market.go
 * @Description:合约行情okexv5版本。实现common.FutureMarket接口
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package okexv5

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"aztecqt/dagger/api/okexv5api"
	"aztecqt/dagger/cex/common"
	"aztecqt/dagger/util"
	"aztecqt/dagger/util/logger"

	"github.com/shopspring/decimal"
)

type FutureMarket struct {
	CommonMarket
	markprice       decimal.Decimal
	maxBuyPrice     decimal.Decimal
	minSellPrice    decimal.Decimal
	fundingRate     decimal.Decimal
	nextFundingRate decimal.Decimal
	fundingTime     time.Time
	nextFundingTime time.Time

	markpriceOK  bool
	priceLimitOK bool
	fundingFeeOK bool
}

func (m *FutureMarket) Init(ex *Exchange, instID, valueCcy, settleCcy string, detailedDepth bool) {
	m.CommonMarket.Init(ex, instID, detailedDepth)
	m.markpriceOK = false
	m.priceLimitOK = false
	m.fundingFeeOK = false

	// 执行频道订阅
	m.subscribe(instID)
	logger.LogImportant(logPrefix, "future market(%s) inited", instID)
}

func (m *FutureMarket) Uninit() {
	// 反订阅所有频道
	m.unsubscribe(m.instId)
	logger.LogImportant(logPrefix, "future market(%s) uninited", m.instId)
}

func (m *FutureMarket) subscribe(instID string) {
	m.CommonMarket.subscribe(instID)

	// 订阅标记价格(20秒超时,服务器保证10秒至少推送一次)
	go func() {
		timeout := time.NewTicker(time.Second * 20)
		s := m.ws.SubscribeMarkPrice(instID, func(resp interface{}) {
			m.onMarkPriceResp(resp.(okexv5api.MarkPriceWsResp).Data[0])
			timeout.Reset(time.Second * 20)
			m.markpriceOK = true
		})

		for {
			<-timeout.C
			m.markpriceOK = false
			s.Reset()
		}
	}()

	// 订阅限价(20秒超时，10秒触发Rest，服务器不保证推送频率)
	go func() {
		timeoutReSub := time.NewTicker(time.Second * 20)
		timeoutREST := time.NewTicker(time.Second * 10)
		s := m.ws.SubscribePriceLimit(instID, func(resp interface{}) {
			m.onPriceLimitResp(resp.(okexv5api.PriceLimitWsResp).Data[0])
			timeoutReSub.Reset(time.Second * 20)
			timeoutREST.Reset(time.Second * 10)
			m.priceLimitOK = true
		})

		for {
			select {
			case <-timeoutREST.C:
				resp, err := okexv5api.GetPriceLimit(instID)
				if err == nil && resp.Code == "0" {
					m.onPriceLimitResp(resp.Data[0])
					timeoutReSub.Reset(time.Second * 20)
					timeoutREST.Reset(time.Second * 10)
					m.priceLimitOK = true
				}
			case <-timeoutReSub.C:
				m.priceLimitOK = false
				s.Reset()
			}
		}
	}()

	if strings.Contains(instID, "SWAP") {
		// 订阅资金费率(180秒超时)
		go func() {
			timeout := time.NewTicker(time.Second * 180)
			s := m.ws.SubscribeFundingrate(instID, func(resp interface{}) {
				m.onFundingRateResp(resp)
				timeout.Reset(time.Second * 180)
				m.fundingFeeOK = true
			})

			for {
				<-timeout.C
				m.fundingFeeOK = false
				s.Reset()
			}
		}()
	} else {
		m.fundingFeeOK = true
	}
}

func (m *FutureMarket) onMarkPriceResp(resp okexv5api.MarkPriceResp) {
	m.markprice = m.AlignPriceNumber(util.String2DecimalPanic(resp.MarkPrice))
}

func (m *FutureMarket) onPriceLimitResp(resp okexv5api.PriceLimitResp) {
	m.minSellPrice = util.String2DecimalPanic(resp.SellLimit)
	m.maxBuyPrice = util.String2DecimalPanic(resp.BuyLimit)
}

func (m *FutureMarket) onFundingRateResp(resp interface{}) {
	r := resp.(okexv5api.FundingRateWsResp)
	m.fundingRate = util.String2DecimalPanic(r.Data[0].FundingRate)
	m.nextFundingRate = util.String2DecimalPanic(r.Data[0].NextFundingRate)
	m.fundingTime = util.ConvetUnix13StrToTimePanic(r.Data[0].FundingTime)
	m.nextFundingTime = util.ConvetUnix13StrToTimePanicUnless(r.Data[0].NextFundingTime, "") // okx的ws中暂时没有这个字段

	m.fundingFeeOK = true
}

// #region 实现common.FutureMarket
func (m *FutureMarket) String() string {
	bb := bytes.Buffer{}
	bb.WriteString(fmt.Sprintf("\nfuture market: %s\n", m.instId))
	bb.WriteString(fmt.Sprintf("price: %s\n", m.latestPrice.String()))
	bb.WriteString(fmt.Sprintf("this funding rate: %s%% \n", m.fundingRate.Mul(decimal.NewFromInt(100)).StringFixed(2)))
	bb.WriteString(fmt.Sprintf("next funding rate: %s%% \n", m.nextFundingRate.Mul(decimal.NewFromInt(100)).StringFixed(2)))
	bb.WriteString("depth:\n")
	bb.WriteString(m.OrderBook().String(5))
	return bb.String()
}

func (m *FutureMarket) Ready() bool {
	return m.depthOK && m.fundingFeeOK && m.markpriceOK && m.priceLimitOK
}

func (m *FutureMarket) ReadyStr() string {
	return fmt.Sprintf("depth_ok:%v fr_ok:%v, mark_price_ok:%v, price_limit_ok:%v", m.depthOK, m.fundingFeeOK, m.markpriceOK, m.priceLimitOK)
}

func (m *FutureMarket) MarkPrice() decimal.Decimal {
	return m.markprice
}

func (m *FutureMarket) FundingInfo() (decimal.Decimal, decimal.Decimal, time.Time, time.Time) {
	return m.fundingRate, m.nextFundingRate, m.fundingTime, m.nextFundingTime
}

func (m *FutureMarket) ValueAmount() decimal.Decimal {
	return m.inst.CtVal
}

func (m *FutureMarket) ValueCurrency() string {
	return m.inst.CtValCcy
}

func (m *FutureMarket) SettlementCurrency() string {
	return m.inst.SettleCcy
}

func (m *FutureMarket) Symbol() string {
	return InstId2Symbol(m.instId)
}

func (m *FutureMarket) ContractType() string {
	return InstId2ContractType(m.instId)
}

func (m *FutureMarket) AlignPrice(price decimal.Decimal, dir common.OrderDir, makeOnly bool) decimal.Decimal {
	aligned := m.CommonMarket.AlignPrice(price, dir, makeOnly)
	if dir == common.OrderDir_Buy && aligned.GreaterThan(m.maxBuyPrice) {
		aligned = m.maxBuyPrice
	} else if dir == common.OrderDir_Sell && aligned.LessThan(m.minSellPrice) {
		aligned = m.minSellPrice
	}
	return aligned
}

// #endregion
