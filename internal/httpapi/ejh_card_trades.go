package httpapi

import (
	"chatgpt-space-merge/internal/ejh"
	"chatgpt-space-merge/internal/store"
	"net/http"
	"time"
)

// Query only the selected issuer card. Each page is a supplier read; saving it
// retains the existing attribution and invalidates changed confirmations.
func (s *Server) queryEJHCardTrades(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Page  int    `json:"page"`
	}
	if decodeJSON(w, r, &in, 4096) != nil {
		return
	}
	zone := time.FixedZone("CST", 8*3600)
	start, e1 := time.ParseInLocation("2006-01-02", in.Start, zone)
	end, e2 := time.ParseInLocation("2006-01-02", in.End, zone)
	end = end.Add(24*time.Hour - time.Second)
	if e1 != nil || e2 != nil || end.Before(start) || end.Sub(start) > 90*24*time.Hour || in.Page < 1 || in.Page > 10000 {
		writeAPI(w, 400, nil, "请选择 1～90 天的日期范围和有效页码")
		return
	}
	if !s.ejh.mu.TryLock() {
		writeAPI(w, 409, nil, "卡台正在同步或执行操作，请稍后重试查询")
		return
	}
	defer s.ejh.mu.Unlock()
	var card ejh.CardRecord
	if err := s.store.EJHGet("card", r.PathValue("id"), &card); err != nil || card.Remote.OrderNo == "" {
		writeAPI(w, 404, nil, "未找到绑定卡台的银行卡")
		return
	}
	cfg, key, err := s.ejhConfig()
	if err != nil {
		writeAPI(w, 400, nil, err.Error())
		return
	}
	var out struct {
		Trades []ejh.Trade `json:"tradeInfos"`
	}
	err = s.ejhClientLocked().Call(r.Context(), cfg, key, "card.trade.info", map[string]any{"orderNo": card.Remote.OrderNo, "tradeTime": start.Format("2006-01-02 15:04:05"), "endTime": end.Format("2006-01-02 15:04:05"), "page": in.Page, "size": 100}, &out)
	if err != nil {
		writeAPI(w, 502, nil, err.Error())
		return
	}
	if out.Trades == nil || len(out.Trades) > 100 {
		writeAPI(w, 502, nil, "卡台未返回有效交易列表，保留已有记录")
		return
	}
	seen := map[string]bool{}
	for _, t := range out.Trades {
		id := store.EJHID("trade", t.OrderNo+"|"+t.TradeNo+"|"+t.Type)
		if t.OrderNo != card.Remote.OrderNo || t.TradeNo == "" || t.Type == "" || seen[id] {
			writeAPI(w, 502, nil, "卡台交易归属或流水编号异常，保留已有记录")
			return
		}
		seen[id] = true
	}
	for i, t := range out.Trades {
		if err = s.store.EJHSaveTrade(card.ID, t); err != nil {
			writeAPI(w, 500, nil, "保存交易明细失败，请重试查询")
			return
		}
		out.Trades[i].Number = maskEJH(t.Number)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAPI(w, 200, map[string]any{"items": out.Trades, "page": in.Page, "has_more": len(out.Trades) == 100, "queried_at": time.Now()}, "")
}
