package device

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yibaiba/hideck/internal/db"
	"github.com/yibaiba/hideck/pkg/logger"
)

var ErrVoWiFiCarrierIdentityMismatch = errors.New("VoWiFi 当前 IMSI 与此卡的运营商身份不符")

type voWiFiCarrierIdentity struct {
	ExpectedPLMN string
	Source       string
	LivePLMN     string
}

func resolveVoWiFiCarrierIdentity(iccid, imsi, mcc, mnc string) (voWiFiCarrierIdentity, error) {
	result := voWiFiCarrierIdentity{LivePLMN: strings.TrimSpace(mcc) + strings.TrimSpace(mnc)}
	iccid = db.CanonicalICCID(iccid)
	imsi = strings.TrimSpace(imsi)
	if iccid == "" || imsi == "" {
		return result, nil
	}
	if result.LivePLMN == "" && strings.HasPrefix(imsi, "20404") {
		result.LivePLMN = "20404"
	}

	policy, err := db.GetCardPolicy(iccid)
	if err != nil && !errors.Is(err, db.ErrCardPolicyNotFound) {
		return result, fmt.Errorf("读取 VoWiFi 卡策略失败: %w", err)
	}
	if err == nil && policy.VoWiFiExpectedPLMN != "" {
		result.ExpectedPLMN = policy.VoWiFiExpectedPLMN
		result.Source = "card_policy"
	} else if strings.HasPrefix(imsi, "20404") {
		// 204/04 may be a legitimate Vodafone SIM. Only this card's own 515/66
		// history makes the ambiguous identity unsafe for automatic VoWiFi.
		history, historyErr := db.ListIMSIsForICCID(iccid)
		if historyErr != nil {
			return result, fmt.Errorf("读取 VoWiFi 卡 IMSI 历史失败: %w", historyErr)
		}
		if hasIMSIPrefix(history, "51566") {
			result.ExpectedPLMN = "51566"
			result.Source = "iccid_imsi_history"
		}
	}
	return result, nil
}

func (v voWiFiCarrierIdentity) mismatch(imsi string) bool {
	if v.ExpectedPLMN == "" || strings.TrimSpace(imsi) == "" {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(imsi), v.ExpectedPLMN) || v.LivePLMN != v.ExpectedPLMN
}

func (v voWiFiCarrierIdentity) check(imsi string) error {
	if !v.mismatch(imsi) {
		return nil
	}
	return fmt.Errorf("%w: 预期 %s，当前 %s（%s）；已停止错误的 ePDG 重试，须恢复卡身份后重试",
		ErrVoWiFiCarrierIdentityMismatch, v.ExpectedPLMN, v.LivePLMN, v.Source)
}

func (p *Pool) stopMismatchedVoWiFi(w *Worker, identity voWiFiCarrierIdentity) {
	if p == nil || w == nil {
		return
	}
	p.clearDesiredVoWiFiRecoverState(w.ID)
	if !p.IsVoWiFiActive(w.ID) && p.GetVoWiFiAppForDevice(w.ID) == nil {
		return
	}
	logger.Warn("停止不匹配的 VoWiFi 身份", "event", "VOWIFI_CARRIER_IDENTITY_MISMATCH",
		"device", w.ID, "expected_plmn", identity.ExpectedPLMN, "live_plmn", identity.LivePLMN,
		"preset_selection_source", identity.Source)
	if err := p.voWiFiHost().Disable(p.Context(), w.ID, "carrier_identity_mismatch", true); err != nil {
		logger.Warn("停止不匹配的 VoWiFi 失败", "device", w.ID, "err", err)
	}
}
