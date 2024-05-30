/*
 * @Author: aztec
 * @Date: 2023-02-27 10:12:42
 * @Description: 币安的现货交易器，实现common.SpotTrader接口
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package binance

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/aztecqt/dagger/util/logger"

	"github.com/aztecqt/dagger/cex/common"
	"github.com/shopspring/decimal"
)

type SpotTrader struct {
	market      *SpotMarket
	exchange    *Exchange
	stratergyId int
	logPrefix   string

	// 余额
	baseBalance  *common.BalanceImpl
	quoteBalance *common.BalanceImpl

	// 订单
	orders   map[string]*SpotOrder // clientId-order
	muOrders sync.RWMutex

	errorlock bool // 出现异常时，锁定订单创建等关键操作
	finished  bool // 结束标志，用来退出某些循环
}

func (t *SpotTrader) Init(ex *Exchange, stratergyId int, m *SpotMarket) {
	t.market = m
	t.exchange = ex
	t.stratergyId = stratergyId
	t.orders = make(map[string]*SpotOrder)
	t.logPrefix = fmt.Sprintf("%s-Trader-%s", logPrefix, m.instId)
	t.finished = false

	// 获取balance指针
	t.baseBalance = ex.spotBalanceMgr.FindBalance(t.market.BaseCurrency())
	t.quoteBalance = ex.spotBalanceMgr.FindBalance(t.market.QuoteCurrency())

	// 订阅order信息
	ex.RegSpotOrderSnapshot(m.instId, func(os OrderSnapshot) {
		var o *SpotOrder = nil
		var ok bool = false

		if os.StratergyId > 0 && os.StratergyId != stratergyId {
			t.errorlock = true
			logger.LogPanic(t.logPrefix, "found order from other stratergy!")
		}

		t.muOrders.RLock()
		o, ok = t.orders[os.ClientOrderID]
		t.muOrders.RUnlock()

		if ok {
			o.onSnapshot(os)
		}
	})

	// 清理finished orders
	go func() {
		for !t.finished {
			t.muOrders.Lock()
			for cid, o := range t.orders {
				if o.Finished {
					delete(t.orders, cid)
				}
			}
			t.muOrders.Unlock()
			time.Sleep(time.Second)
		}
	}()
}

func (t *SpotTrader) Uninit() {
	t.finished = true
	t.exchange.UnregSpotOrderSnapshot(t.market.instId)
	t.market.Uninit()
	logger.LogImportant(logPrefix, "spot trader(%s) uninited", t.market.instId)
}

// 实现common.OrderObserver
func (t *SpotTrader) OnDeal(deal common.Deal) {
	// 订单成交时，记录订单成交造成的权益临时变化
	if deal.O.GetDir() == common.OrderDir_Buy {
		t.baseBalance.RecordTempRights(deal.Amount, deal.UTime)
		t.quoteBalance.RecordTempRights(deal.Amount.Mul(deal.Price).Neg(), deal.UTime)
	} else if deal.O.GetDir() == common.OrderDir_Sell {
		t.baseBalance.RecordTempRights(deal.Amount.Neg(), deal.UTime)
		t.quoteBalance.RecordTempRights(deal.Amount.Mul(deal.Price), deal.UTime)
	}
}

// #region 实现 common.SpotTrader
func (t *SpotTrader) Market() common.CommonMarket {
	return t.market
}

func (t *SpotTrader) SpotMarket() common.SpotMarket {
	return t.market
}

func (t *SpotTrader) String() string {
	bb := bytes.Buffer{}
	bb.WriteString(t.market.String())
	bb.WriteString(fmt.Sprintf("\nspot trader:%s\n", t.market.instId))
	bb.WriteString(fmt.Sprintf("base currency(%s): %v/%v\n", t.market.BaseCurrency(), t.baseBalance.Available(), t.baseBalance.Rights()))
	bb.WriteString(fmt.Sprintf("quote currency(%s): %v/%v\n", t.market.QuoteCurrency(), t.quoteBalance.Available(), t.quoteBalance.Rights()))

	t.muOrders.RLock()
	bb.WriteString(fmt.Sprintf("%d alive orders:\n", len(t.orders)))
	for _, o := range t.orders {
		bb.WriteString(o.String())
	}
	t.muOrders.RUnlock()

	return bb.String()
}

func (t *SpotTrader) Ready() bool {
	baseBalOk, _ := t.baseBalance.Ready()
	quoteBalOk, _ := t.quoteBalance.Ready()
	return t.market.Ready() && baseBalOk && quoteBalOk && exchangeReady && !t.errorlock
}

func (t *SpotTrader) UnreadyReason() string {
	if !t.market.Ready() {
		return t.market.UnreadyReason()
	}

	if ok, reason := t.baseBalance.Ready(); !ok {
		return fmt.Sprintf("base balance(%s) not ready: %s", t.baseBalance.Ccy(), reason)

	}

	if ok, reason := t.quoteBalance.Ready(); !ok {
		return fmt.Sprintf("quote balance(%s) not ready: %s", t.quoteBalance.Ccy(), reason)
	}

	if !exchangeReady {
		return "exchange not ready"
	}

	return ""
}

func (t *SpotTrader) BuyPriceRange() (min, max decimal.Decimal) {
	return decimal.Zero, decimal.NewFromInt(math.MaxInt32)
}

func (t *SpotTrader) SellPriceRange() (min, max decimal.Decimal) {
	return decimal.Zero, decimal.NewFromInt(math.MaxInt32)
}

func (t *SpotTrader) MakeOrder(
	price,
	amount decimal.Decimal,
	dir common.OrderDir,
	makeOnly, reduceOnly bool,
	purpose string,
	obs common.OrderObserver) common.Order {
	if t.Ready() {
		o := new(SpotOrder)
		if o.Init(t, price, amount, dir, makeOnly, purpose) {
			t.muOrders.Lock()
			t.orders[o.CltOrderId.(string)] = o
			t.muOrders.Unlock()
			o.AddObserver(t)   // 先内部处理
			o.AddObserver(obs) // 再外部处理
			o.Go()
			return o
		} else {
			return nil
		}
	} else {
		logger.LogInfo(t.logPrefix, "trader not ready, can't Makeorder. reason=%s", t.UnreadyReason())
		time.Sleep(time.Second)
		return nil
	}
}

func (t *SpotTrader) Orders() []common.Order {
	orders := make([]common.Order, 0, len(t.orders))

	t.muOrders.Lock()
	for _, o := range t.orders {
		orders = append(orders, o)
	}
	t.muOrders.Unlock()

	return orders
}

func (t *SpotTrader) FeeTaker() decimal.Decimal {
	return decimal.Zero
}

func (t *SpotTrader) FeeMaker() decimal.Decimal {
	return decimal.Zero
}

func (t *SpotTrader) AvailableAmount(dir common.OrderDir, price decimal.Decimal) decimal.Decimal {
	if dir == common.OrderDir_Buy {
		// 可买数量为当前可用Quote除以购买价格，向下取整
		amount := t.quoteBalance.Available().Div(price)
		amount = t.market.AlignSize(amount)
		return amount
	} else {
		// 可卖数量为当前可用Base
		return t.baseBalance.Available()
	}
}

func (t *SpotTrader) BaseBalance() common.Balance {
	return t.baseBalance
}

func (t *SpotTrader) QuoteBalance() common.Balance {
	return t.quoteBalance
}

func (t *SpotTrader) AssetId() int {
	return AssetId_Fund
}

// #endregion 实现 common.SpotTrader
