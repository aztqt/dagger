/*
 * @Author: aztec
 * @Date: 2022-10-20 16:00:11
 * @LastEditors: aztec
 * @Description: 币安现货api
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */
package binancespotapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"aztecqt/dagger/api/binanceapi"
	"aztecqt/dagger/util/logger"
	"aztecqt/dagger/util/network"
	"github.com/shopspring/decimal"
)

const rootUrl = "https://api.binance.com"
const restLogPrefix = "binance_spot_rest"

// 获取服务器时间（毫秒数）
var serverTsDelta int64

func GetServerTs() int64 {
	action := "/api/v3/time"
	method := "GET"
	ep := rootUrl + action
	rst, err := network.ParseHttpResult[binanceapi.ServerTime](restLogPrefix, "GetServerTS", ep, method, "", nil, func(resp *http.Response, body []byte) {
		binanceapi.ProcessResponse(resp, body, "spot")
	}, binanceapi.ErrorCallback)
	if err == nil {
		return rst.ServerTime
	} else {
		logger.LogPanic(restLogPrefix, "get server ts failed: %s", err.Error())
		return 0
	}
}

// 获取频率限制
func GetExchangeInfo_RateLimit() (*binanceapi.ExchangeInfo_RateLimit, error) {
	action := "/api/v3/exchangeInfo"
	method := "GET"
	params := url.Values{}
	params.Set("symbol", "BTCUSDT")
	paramsStr := params.Encode()
	action = action + "?" + paramsStr
	ep := rootUrl + action
	rst, err := network.ParseHttpResult[binanceapi.ExchangeInfo_RateLimit](restLogPrefix, "GetExchangeInfo_RateLimit", ep, method, "", nil, func(resp *http.Response, body []byte) {
		binanceapi.ProcessResponse(resp, body, "spot")
	}, binanceapi.ErrorCallback)
	if err == nil && serverTsDelta == 0 {
		serverTsDelta = rst.ServerTime - time.Now().UnixMilli()
	}
	return rst, err
}

// 获取交易对信息
func GetExchangeInfo_Symbols(symbol string) (*binanceapi.ExchangeInfo_Symbols, error) {
	action := "/api/v3/exchangeInfo"
	method := "GET"
	if len(symbol) > 0 {
		params := url.Values{}
		params.Set("symbol", symbol)
		action = action + "?" + params.Encode()
	}
	ep := rootUrl + action

	rst, err := network.ParseHttpResult[binanceapi.ExchangeInfo_Symbols](restLogPrefix, "GetExchangeInfo_Symbols", ep, method, "", nil, func(resp *http.Response, body []byte) {
		binanceapi.ProcessResponse(resp, body, "spot")
	}, binanceapi.ErrorCallback)
	if err == nil && serverTsDelta == 0 {
		serverTsDelta = rst.ServerTime - time.Now().UnixMilli()
	}
	return rst, err
}

// 取K线
// 返回：[[开盘时间，开盘价，最高，最低，收盘价，成交额]]
func GetKline(symbol, interval string, t0, t1 time.Time, limit int) (*binanceapi.KLine, error) {
	action := "/api/v3/klines"
	method := "GET"
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)
	params.Set("limit", fmt.Sprintf("%d", limit))
	if !t0.IsZero() {
		params.Set("startTime", fmt.Sprintf("%d", t0.UnixMilli()))
	}
	if !t1.IsZero() {
		params.Set("endTime", fmt.Sprintf("%d", t1.UnixMilli()))
	}
	paramsStr := params.Encode()
	action = action + "?" + paramsStr
	ep := rootUrl + action
	rst, err := network.ParseHttpResult[binanceapi.KLine](restLogPrefix, "GetKline", ep, method, "", nil, func(resp *http.Response, body []byte) {
		binanceapi.ProcessResponse(resp, body, "spot")
	}, binanceapi.ErrorCallback)

	for i := 0; i < len(*rst); i++ {
		(*rst)[i] = (*rst)[i][:6]
		(*rst)[i][0] = int64((*rst)[i][0].(float64))
	}

	return rst, err
}

// 本地推算服务器时间（毫秒数）
func ServerTs() int64 {
	if serverTsDelta == 0 {
		sts := GetServerTs()
		if sts != 0 {
			serverTsDelta = sts - time.Now().UnixMilli()
		}
	}

	return time.Now().UnixMilli() + serverTsDelta
}

// 现货最新价格
func GetLatestPrice(symbols ...string) (*[]binanceapi.LatestPrice, error) {
	action := "/api/v3/ticker/price"
	method := "GET"
	paramStr := ""
	single := false
	if len(symbols) > 0 {
		params := url.Values{}
		if len(symbols) == 1 {
			params.Set("symbol", symbols[0])
			single = true
		} else {
			d, _ := json.Marshal(symbols)
			symbolsstr := string(d)
			params.Set("symbols", symbolsstr)
		}
		paramStr = params.Encode()
	}

	if len(paramStr) > 0 {
		action = action + "?" + paramStr
	}
	ep := rootUrl + action
	if single {
		rst, err := network.ParseHttpResult[binanceapi.LatestPrice](restLogPrefix, "GetSpotLatestPrice", ep, method, "", nil, func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
		if err == nil {
			rst.Ts = ServerTs()
			respArry := make([]binanceapi.LatestPrice, 0)
			respArry = append(respArry, *rst)
			return &respArry, nil
		} else {
			return nil, err
		}
	} else {
		rst, err := network.ParseHttpResult[[]binanceapi.LatestPrice](restLogPrefix, "GetSpotLatestPrice", ep, method, "", nil, func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
		ts := ServerTs()
		for i := range *rst {
			(*rst)[i].Ts = ts
		}
		return rst, err
	}
}

// 现货买一卖一价格
func GetBookTicker(symbols ...string) (*[]binanceapi.BookTicker, error) {
	action := "/api/v3/ticker/bookTicker"
	method := "GET"
	paramStr := ""
	single := false
	if len(symbols) > 0 {
		params := url.Values{}
		if len(symbols) == 1 {
			params.Set("symbol", symbols[0])
			single = true
		} else {
			d, _ := json.Marshal(symbols)
			symbolsstr := string(d)
			params.Set("symbols", symbolsstr)
		}
		paramStr = params.Encode()
	}

	if len(paramStr) > 0 {
		action = action + "?" + paramStr
	}
	ep := rootUrl + action
	if single {
		rst, err := network.ParseHttpResult[binanceapi.BookTicker](restLogPrefix, "GetSpotBookTicker", ep, method, "", nil, func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
		if err == nil {
			rst.Ts = ServerTs()
			respArry := make([]binanceapi.BookTicker, 0)
			respArry = append(respArry, *rst)
			return &respArry, nil
		} else {
			return nil, err
		}

	} else {
		rst, err := network.ParseHttpResult[[]binanceapi.BookTicker](restLogPrefix, "GetSpotBookTicker", ep, method, "", nil, func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
		ts := ServerTs()
		for i := range *rst {
			(*rst)[i].Ts = ts
		}
		return rst, err
	}
}

// ListenKey(UserDataStream)管理
func GetListenKey() (*binanceapi.ListenKeyResponse, error) {
	action := "/api/v3/userDataStream"
	method := "POST"
	header := binanceapi.SignerIns.HeaderWithApiKey()
	ep := fmt.Sprintf("%s%s", rootUrl, action)

	rest, err := network.ParseHttpResult[binanceapi.ListenKeyResponse](
		restLogPrefix,
		"GetListenKey",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}

func KeepListenKey(listenKey string) (*binanceapi.ErrorMessage, error) {
	action := "/api/v3/userDataStream"
	method := "PUT"

	params := url.Values{}
	params.Set("listenKey", listenKey)
	header := binanceapi.SignerIns.HeaderWithApiKey()
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, params.Encode())

	rest, err := network.ParseHttpResult[binanceapi.ErrorMessage](
		restLogPrefix,
		"KeepListenKey",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}

// 获取现货账户权益
func GetAccountInfo() (*binanceapi.AccountInfo, error) {
	action := "/api/v3/account"
	method := "GET"

	// 参数（无业务参数）
	params := url.Values{}
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	rest, err := network.ParseHttpResult[binanceapi.AccountInfo](
		restLogPrefix,
		"GetAccountInfo",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}

// 下单
// 订单方向(side)：BUY/SELL
// 订单类型(type)
// LIMIT 限价单/MARKET 市价单
// STOP_LOSS 止损单/STOP_LOSS_LIMIT 限价止损单/TAKE_PROFIT 止盈单/TAKE_PROFIT_LIMIT 限价止盈单
// LIMIT_MAKER 限价只挂单
func MakeOrder(symbol, side, orderType, clientOrderID string, price, quantity decimal.Decimal) (*binanceapi.MakeOrderResponse_Ack, error) {
	action := "/api/v3/order"
	method := "POST"

	// 参数
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", side)
	params.Set("type", orderType)
	params.Set("newClientOrderId", clientOrderID)
	params.Set("price", price.String())
	params.Set("quantity", quantity.String())
	params.Set("timeInForce", "GTC")
	params.Set("newOrderRespType", "ACK") // ACK/RESULT/FULL
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	rest, err := network.ParseHttpResult[binanceapi.MakeOrderResponse_Ack](
		restLogPrefix,
		"MakeOrder",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}

// 撤单
// 有orderId则优先使用orderId
func CancelOrder(symbol string, orderId int64, clientOrderId string) (*binanceapi.CancelOrderResponse, error) {
	action := "/api/v3/order"
	method := "DELETE"

	// 参数
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderId > 0 {
		params.Set("orderId", fmt.Sprintf("%d", orderId))
	} else if len(clientOrderId) > 0 {
		params.Set("origClientOrderId", clientOrderId)
	} else {
		logger.LogPanic(restLogPrefix, "CancelOrder-no orderId and no clientOrderId")
	}
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	rest, err := network.ParseHttpResult[binanceapi.CancelOrderResponse](
		restLogPrefix,
		"CancelOrder",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}

// 撤销某一交易对下的所有订单
func CancelOpenOrders(symbol string) (*binanceapi.CancelOpenOrdersResponse, *binanceapi.ErrorMessage, error) {
	action := "/api/v3/openOrders"
	method := "DELETE"

	// 参数
	params := url.Values{}
	params.Set("symbol", symbol)
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	var errmsg *binanceapi.ErrorMessage
	rest, err := network.ParseHttpResult[binanceapi.CancelOpenOrdersResponse](
		restLogPrefix,
		"CancelOpenOrders",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			errmsg = binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	if errmsg != nil {
		err = nil
	}

	return rest, errmsg, err
}

// 查询订单
func GetOrder(symbol string, orderId int64, clientOrderId string) (*binanceapi.GetOrderResponse, error) {
	action := "/api/v3/order"
	method := "GET"

	// 参数
	params := url.Values{}
	params.Set("symbol", symbol)
	if orderId > 0 {
		params.Set("orderId", fmt.Sprintf("%d", orderId))
	} else if len(clientOrderId) > 0 {
		params.Set("origClientOrderId", clientOrderId)
	} else {
		logger.LogPanic(restLogPrefix, "GetOrder-no orderId and no clientOrderId")
	}

	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	resp, err := network.ParseHttpResult[binanceapi.GetOrderResponse](
		restLogPrefix,
		"GetOrder",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	resp.LocalTime = time.Now()
	return resp, err
}

// 查询所有挂单
// symbol不指定，则会返回所有交易对的挂单，但成本为40。指定的话成本为3
func GetOpenOrders(symbol string) (*binanceapi.GetOpenOrdersResponse, *binanceapi.ErrorMessage, error) {
	action := "/api/v3/openOrders"
	method := "GET"

	// 参数
	params := url.Values{}
	if len(symbol) > 0 {
		params.Set("symbol", symbol)
	}
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	var errmsg *binanceapi.ErrorMessage
	rest, err := network.ParseHttpResult[binanceapi.GetOpenOrdersResponse](
		restLogPrefix,
		"GetOpenOrders",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			errmsg = binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	if errmsg != nil {
		err = nil
	}

	return rest, errmsg, err
}

// 测试接口
func GetWalletSystemStatus() {
	action := "/sapi/v1/system/status"
	method := "GET"

	// 参数
	params := url.Values{}
	header, paramstr, _ := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	network.ParseHttpResult[interface{}](
		restLogPrefix,
		"GetOpenOrders",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
}

func WalletDust() {
	action := "/sapi/v1/asset/dust"
	method := "POST"

	// 参数
	params := url.Values{}
	params.Add("asset", "XRP")
	header, paramstr, _ := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	network.ParseHttpResult[interface{}](
		restLogPrefix,
		"GetOpenOrders",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)
}

// 测试接口
func MakeMarginOrder(symbol, side, orderType, clientOrderID string, price, quantity decimal.Decimal) (*binanceapi.MakeOrderResponse_Ack, error) {
	action := "/sapi/v1/margin/order"
	method := "POST"

	// 参数
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("side", side)
	params.Set("type", orderType)
	params.Set("newClientOrderId", clientOrderID)
	params.Set("price", price.String())
	params.Set("quantity", quantity.String())
	params.Set("timeInForce", "GTC")
	params.Set("newOrderRespType", "ACK") // ACK/RESULT/FULL
	header, paramstr, err := binanceapi.SignerIns.Sign(params)
	ep := fmt.Sprintf("%s%s?%s", rootUrl, action, paramstr)

	rest, err := network.ParseHttpResult[binanceapi.MakeOrderResponse_Ack](
		restLogPrefix,
		"MakeOrder",
		ep,
		method,
		"",
		header,
		func(resp *http.Response, body []byte) {
			binanceapi.ProcessResponse(resp, body, "spot")
		}, binanceapi.ErrorCallback)

	return rest, err
}
