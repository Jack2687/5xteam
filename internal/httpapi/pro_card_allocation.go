package httpapi

import (
	"context"
	"fmt"
	"time"

	"chatgpt-space-merge/internal/gptpay"
)

// Serialize selection, optional issuance and reservation across manual, auto
// and scheduled starts. The database still enforces physical-card occupancy.
func (s *Server) reserveProStartCard(ctx context.Context, email, selected string, autoOpen, enforceMaximum bool) (gptpay.Card, gptpay.CardSecret, error) {
	s.proCardAdmissionMu.Lock()
	defer s.proCardAdmissionMu.Unlock()
	var card gptpay.Card
	var secret gptpay.CardSecret
	if err := ctx.Err(); err != nil {
		return card, secret, err
	}
	if enforceMaximum {
		pro, _, _, err := s.store.ProSettings()
		if err != nil {
			return card, secret, err
		}
		capacity, err := s.store.ProCapacity()
		if err != nil {
			return card, secret, err
		}
		if capacity.OpenedUnmerged+capacity.Pending >= pro.MaxUnmerged {
			return card, secret, fmt.Errorf("已开通和在途账号已达到最大未合并数")
		}
	}
	var err error
	if selected != "" {
		card, secret, err = s.store.GPTPayCard(selected)
		if err != nil || !card.Enabled {
			return card, secret, fmt.Errorf("请选择已启用的银行卡")
		}
		if err = gptpay.ValidateCard(secret, time.Now()); err != nil {
			return card, secret, err
		}
		if card.Source == "ejh" && (card.IssuerLocked || (card.IssuerStatus != "1" && card.IssuerStatus != "3")) {
			return card, secret, fmt.Errorf("卡台卡片未激活、冻结或状态不可用")
		}
		if card.IssuerEnd != "" {
			end, e := time.ParseInLocation("2006-01-02 15:04:05", card.IssuerEnd, time.FixedZone("CST", 8*3600))
			if e != nil || !end.After(time.Now()) {
				return card, secret, fmt.Errorf("卡台卡片已过期或到期时间无效")
			}
		}
		if !card.Available {
			// A previously selectable EJH card may have been claimed by another
			// task. Only this capacity/occupancy case permits automatic fallback;
			// frozen, expired and disabled cards are not silently substituted.
			if autoOpen && card.CardType == "ejh" && (card.RemainingAccounts == 0 || (card.SingleActiveAccount && card.ActiveAccounts > 0)) {
				selected = ""
			} else if card.SingleActiveAccount && card.ActiveAccounts > 0 {
				return card, secret, fmt.Errorf("EJH 银行卡正在被 %s 占用（一卡同时一个号）；请启用 EJH 自动补卡或等待移出成功", card.ActiveEmail)
			} else {
				return card, secret, fmt.Errorf("银行卡没有剩余开通名额或状态不可用")
			}
		}
	}
	if selected == "" {
		if !autoOpen {
			return card, secret, fmt.Errorf("请选择银行卡，或启用 EJH 自动补卡")
		}
		card, err = s.ensureEJHCard(ctx)
		if err != nil {
			return card, secret, err
		}
		card, secret, err = s.store.GPTPayCard(card.ID)
		if err != nil {
			return card, secret, err
		}
	}
	if err = ctx.Err(); err != nil {
		return card, secret, err
	}
	err = s.store.ReserveProCard(email, card.ID, enforceMaximum)
	return card, secret, err
}
