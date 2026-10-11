package ocpp

import "sort"

// OCPP 2.1 messages. Their payloads are checked against the OCA part 3
// JSON schemas (schema.go); these tables only say who sends what.

// in21 are the CALLs a 2.1 charging station sends to the CSMS.
var in21 = setOf(
	// shared with 2.0.1
	"Authorize", "BootNotification", "ClearedChargingLimit", "DataTransfer", "FirmwareStatusNotification",
	"Get15118EVCertificate", "GetCertificateStatus", "Heartbeat", "LogStatusNotification", "MeterValues",
	"NotifyChargingLimit", "NotifyCustomerInformation", "NotifyDisplayMessages", "NotifyEVChargingNeeds",
	"NotifyEVChargingSchedule", "NotifyEvent", "NotifyMonitoringReport", "NotifyReport",
	"PublishFirmwareStatusNotification", "ReportChargingProfiles", "ReservationStatusUpdate",
	"SecurityEventNotification", "SignCertificate", "StatusNotification", "TransactionEvent",
	// new in 2.1
	"BatterySwap", "ClosePeriodicEventStream", "GetCertificateChainStatus", "NotifyDERAlarm",
	"NotifyDERStartStop", "NotifyPriorityCharging", "NotifySettlement", "OpenPeriodicEventStream",
	"PullDynamicScheduleUpdate", "ReportDERControl", "VatNumberValidation",
)

// out21 are the CALLs the CSMS sends to a 2.1 charging station.
var out21 = setOf(
	// shared with 2.0.1
	"CancelReservation", "CertificateSigned", "ChangeAvailability", "ClearCache", "ClearChargingProfile",
	"ClearDisplayMessage", "ClearVariableMonitoring", "CostUpdated", "CustomerInformation", "DataTransfer",
	"DeleteCertificate", "GetBaseReport", "GetChargingProfiles", "GetCompositeSchedule", "GetDisplayMessages",
	"GetInstalledCertificateIds", "GetLocalListVersion", "GetLog", "GetMonitoringReport", "GetReport",
	"GetTransactionStatus", "GetVariables", "InstallCertificate", "PublishFirmware", "RequestStartTransaction",
	"RequestStopTransaction", "ReserveNow", "Reset", "SendLocalList", "SetChargingProfile", "SetDisplayMessage",
	"SetMonitoringBase", "SetMonitoringLevel", "SetNetworkProfile", "SetVariableMonitoring", "SetVariables",
	"TriggerMessage", "UnlockConnector", "UnpublishFirmware", "UpdateFirmware",
	// new in 2.1
	"AFRRSignal", "AdjustPeriodicEventStream", "ChangeTransactionTariff", "ClearDERControl", "ClearTariffs",
	"GetDERControl", "GetPeriodicEventStream", "GetTariffs", "NotifyAllowedEnergyTransfer",
	"NotifyWebPaymentStarted", "RequestBatterySwap", "SetDERControl", "SetDefaultTariff",
	"UpdateDynamicSchedule", "UsePriorityCharging",
)

// sendActions21 are SEND messages (part 4 clause 4.2.4): a charging station
// sends them unconfirmed; they have one schema, without a suffix.
var sendActions21 = setOf("NotifyPeriodicEventStream")

// In21, Out21 and SendActions21 are the OCPP 2.1 actions by direction:
// CALLs a charging station sends, CALLs the CSMS sends, and SEND messages
// (unconfirmed, from a charging station).
var (
	In21          = in21
	Out21         = out21
	SendActions21 = sendActions21
)

func setOf(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
