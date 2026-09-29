package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yibaiba/hideck/internal/backend"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/db"
	"github.com/yibaiba/hideck/internal/vowifihost"
)

func TestDITOHistoryStopsFlippedIMSIWithoutReclassifyingVodafone(t *testing.T) {
	openDeviceTestDB(t)
	iccid := "8963660000000000001"
	if err := db.UpsertSIMCard(iccid, "515660000000001", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSIMCard(iccid, "204040000000001", "", "", nil); err != nil {
		t.Fatal(err)
	}
	identity, err := resolveVoWiFiCarrierIdentity(iccid+"F", "204040000000001", "204", "04")
	if err != nil || identity.ExpectedPLMN != "51566" || identity.Source != "iccid_imsi_history" ||
		!errors.Is(identity.check("204040000000001"), ErrVoWiFiCarrierIdentityMismatch) {
		t.Fatalf("flipped identity=%+v err=%v", identity, err)
	}
	stale, err := resolveVoWiFiCarrierIdentity(iccid, "204040000000001", "515", "66")
	if err != nil || !errors.Is(stale.check("204040000000001"), ErrVoWiFiCarrierIdentityMismatch) {
		t.Fatalf("stale SIM home cache must not bypass live IMSI: identity=%+v err=%v", stale, err)
	}
	other, err := resolveVoWiFiCarrierIdentity("8944000000000000002", "204040000000002", "204", "04")
	if err != nil || other.check("204040000000002") != nil {
		t.Fatalf("genuine Vodafone identity=%+v err=%v", other, err)
	}
}

func TestDITOFlippedIdentityRejectedBeforePreparingEPDG(t *testing.T) {
	openDeviceTestDB(t)
	iccid := "8963660000000000001"
	for _, imsi := range []string{"515660000000001", "204040000000001"} {
		if err := db.UpsertSIMCard(iccid, imsi, "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	p := NewPool(&config.Config{})
	b := &vowifiLiveIdentityBackendStub{
		workerSMSCBackendStub: workerSMSCBackendStub{
			workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI, nativeMCC: "204", nativeMNC: "04"},
		},
		liveIMSI: "204040000000001",
	}
	w := &Worker{ID: "dito", Backend: b}
	w.state.Identity.ICCID = iccid
	w.state.Identity.IMSI = b.liveIMSI
	w.state.Identity.IMEI = "861234567890123"
	if _, err := p.buildVoWiFiStartProfile(w, "trace-dito"); !errors.Is(err, ErrVoWiFiCarrierIdentityMismatch) {
		t.Fatalf("buildVoWiFiStartProfile() err=%v, want identity mismatch", err)
	}

	b.liveIMSI = "515660000000001"
	b.nativeMCC, b.nativeMNC = "515", "66"
	profile, err := p.buildVoWiFiStartProfile(w, "trace-dito-restored")
	if err != nil || profile.MCC != "515" || profile.MNC != "66" || profile.IMSI != b.liveIMSI {
		t.Fatalf("restored profile=%+v err=%v", profile, err)
	}
}

func TestManualExpectedPLMNSuppressesDesiredRecovery(t *testing.T) {
	openDeviceTestDB(t)
	p := newDesiredVoWiFiTestPool(t, "dito", true, "204040000000001")
	w := p.GetWorker("dito")
	w.state.Identity.NativeMCC = "204"
	w.state.Identity.NativeMNC = "04"
	pol := db.DefaultCardPolicy(w.CurrentICCID())
	pol.VoWiFiExpectedPLMN = "51566"
	if err := db.UpsertCardPolicy(pol); err != nil {
		t.Fatal(err)
	}
	commands := make(chan vowifihost.LifecycleCommand, 1)
	p.voWiFiHost().LifecycleControllerForTest().TestRun = func(_ context.Context, cmd vowifihost.LifecycleCommand) error {
		commands <- cmd
		return nil
	}
	p.reconcileDesiredVoWiFiOnce(time.Now())
	assertNoRecoverCommand(t, commands)
	if p.shouldReconcileVoWiFi(w) {
		t.Fatal("mismatched card identity should not requeue recovery")
	}
}
