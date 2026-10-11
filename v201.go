package ocpp

import "encoding/json"

// OCPP 2.0.1 JSON message types: field names, required properties,
// string lengths and enumerations as the 2.0.1 JSON schemas define them.
// Every object may also carry "customData" (validate allows it).

type StatusInfo struct {
	ReasonCode     string `json:"reasonCode" ocpp:"req,max=20"`
	AdditionalInfo string `json:"additionalInfo,omitempty" ocpp:"max=512"`
}

type AdditionalInfo struct {
	AdditionalIDToken string `json:"additionalIdToken" ocpp:"req,max=36"`
	Type              string `json:"type" ocpp:"req,max=50"`
}

type IDToken struct {
	IDToken        string           `json:"idToken" ocpp:"req,max=36"`
	Type           string           `json:"type" ocpp:"req,enum=Central|eMAID|ISO14443|ISO15693|KeyCode|Local|MacAddress|NoAuthorization"`
	AdditionalInfo []AdditionalInfo `json:"additionalInfo,omitempty" ocpp:"minItems=1"`
}

type IDTokenInfo struct {
	Status              string          `json:"status" ocpp:"req,enum=Accepted|Blocked|ConcurrentTx|Expired|Invalid|NoCredit|NotAllowedTypeEVSE|NotAtThisLocation|NotAtThisTime|Unknown"`
	CacheExpiryDateTime string          `json:"cacheExpiryDateTime,omitempty" ocpp:"dt"`
	ChargingPriority    *int            `json:"chargingPriority,omitempty"`
	Language1           string          `json:"language1,omitempty" ocpp:"max=8"`
	Language2           string          `json:"language2,omitempty" ocpp:"max=8"`
	EVSEID              []int           `json:"evseId,omitempty" ocpp:"minItems=1"`
	GroupIDToken        *IDToken        `json:"groupIdToken,omitempty"`
	PersonalMessage     json.RawMessage `json:"personalMessage,omitempty"`
}

type EVSE struct {
	ID          int  `json:"id" ocpp:"req,min=0"`
	ConnectorID *int `json:"connectorId,omitempty"`
}

type UnitOfMeasure struct {
	Unit       string `json:"unit,omitempty" ocpp:"max=20"`
	Multiplier *int   `json:"multiplier,omitempty"`
}

type SampledValue201 struct {
	Value            float64         `json:"value" ocpp:"req"`
	Context          string          `json:"context,omitempty" ocpp:"enum=Interruption.Begin|Interruption.End|Other|Sample.Clock|Sample.Periodic|Transaction.Begin|Transaction.End|Trigger"`
	Measurand        string          `json:"measurand,omitempty" ocpp:"enum=Current.Export|Current.Import|Current.Offered|Energy.Active.Export.Register|Energy.Active.Import.Register|Energy.Reactive.Export.Register|Energy.Reactive.Import.Register|Energy.Active.Export.Interval|Energy.Active.Import.Interval|Energy.Active.Net|Energy.Reactive.Export.Interval|Energy.Reactive.Import.Interval|Energy.Reactive.Net|Energy.Apparent.Net|Energy.Apparent.Import|Energy.Apparent.Export|Frequency|Power.Active.Export|Power.Active.Import|Power.Factor|Power.Offered|Power.Reactive.Export|Power.Reactive.Import|SoC|Voltage"`
	Phase            string          `json:"phase,omitempty" ocpp:"enum=L1|L2|L3|N|L1-N|L2-N|L3-N|L1-L2|L2-L3|L3-L1"`
	Location         string          `json:"location,omitempty" ocpp:"enum=Body|Cable|EV|Inlet|Outlet"`
	SignedMeterValue json.RawMessage `json:"signedMeterValue,omitempty"`
	UnitOfMeasure    *UnitOfMeasure  `json:"unitOfMeasure,omitempty"`
}

type MeterValue201 struct {
	Timestamp    string            `json:"timestamp" ocpp:"req,dt"`
	SampledValue []SampledValue201 `json:"sampledValue" ocpp:"req,minItems=1"`
}

type ChargingSchedulePeriod201 struct {
	StartPeriod  int     `json:"startPeriod" ocpp:"req"`
	Limit        float64 `json:"limit" ocpp:"req"`
	NumberPhases *int    `json:"numberPhases,omitempty"`
	PhaseToUse   *int    `json:"phaseToUse,omitempty"`
}

type ChargingSchedule201 struct {
	ID                     int                         `json:"id" ocpp:"req"`
	StartSchedule          string                      `json:"startSchedule,omitempty" ocpp:"dt"`
	Duration               *int                        `json:"duration,omitempty"`
	ChargingRateUnit       string                      `json:"chargingRateUnit" ocpp:"req,enum=W|A"`
	ChargingSchedulePeriod []ChargingSchedulePeriod201 `json:"chargingSchedulePeriod" ocpp:"req,minItems=1,maxItems=1024"`
	MinChargingRate        *float64                    `json:"minChargingRate,omitempty"`
	SalesTariff            json.RawMessage             `json:"salesTariff,omitempty"`
}

type ChargingProfile201 struct {
	ID                     int                   `json:"id" ocpp:"req"`
	StackLevel             int                   `json:"stackLevel" ocpp:"req,min=0"`
	ChargingProfilePurpose string                `json:"chargingProfilePurpose" ocpp:"req,enum=ChargingStationExternalConstraints|ChargingStationMaxProfile|TxDefaultProfile|TxProfile"`
	ChargingProfileKind    string                `json:"chargingProfileKind" ocpp:"req,enum=Absolute|Recurring|Relative"`
	RecurrencyKind         string                `json:"recurrencyKind,omitempty" ocpp:"enum=Daily|Weekly"`
	ValidFrom              string                `json:"validFrom,omitempty" ocpp:"dt"`
	ValidTo                string                `json:"validTo,omitempty" ocpp:"dt"`
	TransactionID          string                `json:"transactionId,omitempty" ocpp:"max=36"`
	ChargingSchedule       []ChargingSchedule201 `json:"chargingSchedule" ocpp:"req,minItems=1,maxItems=3"`
}

type Component struct {
	Name     string `json:"name" ocpp:"req,max=50"`
	Instance string `json:"instance,omitempty" ocpp:"max=50"`
	EVSE     *EVSE  `json:"evse,omitempty"`
}

type Variable struct {
	Name     string `json:"name" ocpp:"req,max=50"`
	Instance string `json:"instance,omitempty" ocpp:"max=50"`
}

// ---- charging station → CSMS ----

type Modem struct {
	ICCID string `json:"iccid,omitempty" ocpp:"max=20"`
	IMSI  string `json:"imsi,omitempty" ocpp:"max=20"`
}

type ChargingStation struct {
	SerialNumber    string `json:"serialNumber,omitempty" ocpp:"max=25"`
	Model           string `json:"model" ocpp:"req,max=20"`
	Modem           *Modem `json:"modem,omitempty"`
	VendorName      string `json:"vendorName" ocpp:"req,max=50"`
	FirmwareVersion string `json:"firmwareVersion,omitempty" ocpp:"max=50"`
}

type BootNotification201 struct {
	ChargingStation ChargingStation `json:"chargingStation" ocpp:"req"`
	Reason          string          `json:"reason" ocpp:"req,enum=ApplicationReset|FirmwareUpdate|LocalReset|PowerUp|RemoteReset|ScheduledReset|Triggered|Unknown|Watchdog"`
}

type BootNotification201Conf struct {
	CurrentTime string      `json:"currentTime" ocpp:"req,dt"`
	Interval    int         `json:"interval" ocpp:"req"`
	Status      string      `json:"status" ocpp:"req,enum=Accepted|Pending|Rejected"`
	StatusInfo  *StatusInfo `json:"statusInfo,omitempty"`
}

type StatusNotification201 struct {
	Timestamp       string `json:"timestamp" ocpp:"req,dt"`
	ConnectorStatus string `json:"connectorStatus" ocpp:"req,enum=Available|Occupied|Reserved|Unavailable|Faulted"`
	EVSEID          int    `json:"evseId" ocpp:"req,min=0"`
	ConnectorID     int    `json:"connectorId" ocpp:"req,min=0"`
}

type Authorize201 struct {
	IDToken                     IDToken           `json:"idToken" ocpp:"req"`
	Certificate                 string            `json:"certificate,omitempty" ocpp:"max=5500"`
	ISO15118CertificateHashData []OCSPRequestData `json:"iso15118CertificateHashData,omitempty" ocpp:"minItems=1,maxItems=4"`
}

type Authorize201Conf struct {
	IDTokenInfo       IDTokenInfo `json:"idTokenInfo" ocpp:"req"`
	CertificateStatus string      `json:"certificateStatus,omitempty" ocpp:"enum=Accepted|SignatureError|CertificateExpired|CertificateRevoked|NoCertificateAvailable|CertChainError|ContractCancelled"`
}

type TransactionInfo struct {
	TransactionID     string `json:"transactionId" ocpp:"req,max=36"`
	ChargingState     string `json:"chargingState,omitempty" ocpp:"enum=Charging|EVConnected|SuspendedEV|SuspendedEVSE|Idle"`
	TimeSpentCharging *int   `json:"timeSpentCharging,omitempty"`
	StoppedReason     string `json:"stoppedReason,omitempty" ocpp:"enum=DeAuthorized|EmergencyStop|EnergyLimitReached|EVDisconnected|GroundFault|ImmediateReset|Local|LocalOutOfCredit|MasterPass|Other|OvercurrentFault|PowerLoss|PowerQuality|Reboot|Remote|SOCLimitReached|StoppedByEV|TimeLimitReached|Timeout"`
	RemoteStartID     *int   `json:"remoteStartId,omitempty"`
}

type TransactionEvent201 struct {
	EventType          string          `json:"eventType" ocpp:"req,enum=Ended|Started|Updated"`
	Timestamp          string          `json:"timestamp" ocpp:"req,dt"`
	TriggerReason      string          `json:"triggerReason" ocpp:"req,enum=Authorized|CablePluggedIn|ChargingRateChanged|ChargingStateChanged|Deauthorized|EnergyLimitReached|EVCommunicationLost|EVConnectTimeout|MeterValueClock|MeterValuePeriodic|TimeLimitReached|Trigger|UnlockCommand|StopAuthorized|EVDeparted|EVDetected|RemoteStop|RemoteStart|AbnormalCondition|SignedDataReceived|ResetCommand"`
	SeqNo              int             `json:"seqNo" ocpp:"req,min=0"`
	Offline            bool            `json:"offline,omitempty"`
	NumberOfPhasesUsed *int            `json:"numberOfPhasesUsed,omitempty"`
	CableMaxCurrent    *int            `json:"cableMaxCurrent,omitempty"`
	ReservationID      *int            `json:"reservationId,omitempty"`
	TransactionInfo    TransactionInfo `json:"transactionInfo" ocpp:"req"`
	IDToken            *IDToken        `json:"idToken,omitempty"`
	EVSE               *EVSE           `json:"evse,omitempty"`
	MeterValue         []MeterValue201 `json:"meterValue,omitempty" ocpp:"minItems=1"`
}

type TransactionEvent201Conf struct {
	TotalCost              *float64        `json:"totalCost,omitempty"`
	ChargingPriority       *int            `json:"chargingPriority,omitempty"`
	IDTokenInfo            *IDTokenInfo    `json:"idTokenInfo,omitempty"`
	UpdatedPersonalMessage json.RawMessage `json:"updatedPersonalMessage,omitempty"`
}

type MeterValues201 struct {
	EVSEID     int             `json:"evseId" ocpp:"req,min=0"`
	MeterValue []MeterValue201 `json:"meterValue" ocpp:"req,minItems=1"`
}

type EventData struct {
	EventID               int       `json:"eventId" ocpp:"req"`
	Timestamp             string    `json:"timestamp" ocpp:"req,dt"`
	Trigger               string    `json:"trigger" ocpp:"req,enum=Alerting|Delta|Periodic"`
	Cause                 *int      `json:"cause,omitempty"`
	ActualValue           string    `json:"actualValue" ocpp:"req,max=2500"`
	TechCode              string    `json:"techCode,omitempty" ocpp:"max=50"`
	TechInfo              string    `json:"techInfo,omitempty" ocpp:"max=500"`
	Cleared               *bool     `json:"cleared,omitempty"`
	TransactionID         string    `json:"transactionId,omitempty" ocpp:"max=36"`
	Component             Component `json:"component" ocpp:"req"`
	VariableMonitoringID  *int      `json:"variableMonitoringId,omitempty"`
	EventNotificationType string    `json:"eventNotificationType" ocpp:"req,enum=HardWiredNotification|HardWiredMonitor|PreconfiguredMonitor|CustomMonitor"`
	Variable              Variable  `json:"variable" ocpp:"req"`
}

type NotifyEvent201 struct {
	GeneratedAt string      `json:"generatedAt" ocpp:"req,dt"`
	Tbc         bool        `json:"tbc,omitempty"`
	SeqNo       int         `json:"seqNo" ocpp:"req,min=0"`
	EventData   []EventData `json:"eventData" ocpp:"req,minItems=1"`
}

type SecurityEventNotification201 struct {
	Type      string `json:"type" ocpp:"req,max=50"`
	Timestamp string `json:"timestamp" ocpp:"req,dt"`
	TechInfo  string `json:"techInfo,omitempty" ocpp:"max=255"`
}

type FirmwareStatusNotification201 struct {
	Status    string `json:"status" ocpp:"req,enum=Downloaded|DownloadFailed|Downloading|DownloadScheduled|DownloadPaused|Idle|InstallationFailed|Installing|Installed|InstallRebooting|InstallScheduled|InstallVerificationFailed|InvalidSignature|SignatureVerified"`
	RequestID *int   `json:"requestId,omitempty"`
}

type LogStatusNotification201 struct {
	Status    string `json:"status" ocpp:"req,enum=BadMessage|Idle|NotSupportedOperation|PermissionDenied|Uploaded|UploadFailure|Uploading|AcceptedCanceled"`
	RequestID *int   `json:"requestId,omitempty"`
}

type DataTransfer201 struct {
	MessageID string          `json:"messageId,omitempty" ocpp:"max=50"`
	Data      json.RawMessage `json:"data,omitempty"`
	VendorID  string          `json:"vendorId" ocpp:"req,max=255"`
}

type DataTransfer201Conf struct {
	Status     string          `json:"status" ocpp:"req,enum=Accepted|Rejected|UnknownMessageId|UnknownVendorId"`
	StatusInfo *StatusInfo     `json:"statusInfo,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// ---- CSMS → charging station ----

type StatusConf201 struct {
	Status     string      `json:"status" ocpp:"req"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type RequestStartTransaction201 struct {
	EVSEID          *int                `json:"evseId,omitempty" ocpp:"min=1"`
	RemoteStartID   int                 `json:"remoteStartId" ocpp:"req"`
	IDToken         IDToken             `json:"idToken" ocpp:"req"`
	ChargingProfile *ChargingProfile201 `json:"chargingProfile,omitempty"`
	GroupIDToken    *IDToken            `json:"groupIdToken,omitempty"`
}

type RequestStartTransaction201Conf struct {
	Status        string      `json:"status" ocpp:"req,enum=Accepted|Rejected"`
	StatusInfo    *StatusInfo `json:"statusInfo,omitempty"`
	TransactionID string      `json:"transactionId,omitempty" ocpp:"max=36"`
}

type RequestStopTransaction201 struct {
	TransactionID string `json:"transactionId" ocpp:"req,max=36"`
}

type Reset201 struct {
	Type   string `json:"type" ocpp:"req,enum=Immediate|OnIdle"`
	EVSEID *int   `json:"evseId,omitempty"`
}

type ChangeAvailability201 struct {
	EVSE              *EVSE  `json:"evse,omitempty"`
	OperationalStatus string `json:"operationalStatus" ocpp:"req,enum=Inoperative|Operative"`
}

type SetVariableData struct {
	AttributeType  string    `json:"attributeType,omitempty" ocpp:"enum=Actual|Target|MinSet|MaxSet"`
	AttributeValue string    `json:"attributeValue" ocpp:"req,max=1000"`
	Component      Component `json:"component" ocpp:"req"`
	Variable       Variable  `json:"variable" ocpp:"req"`
}

type SetVariables201 struct {
	SetVariableData []SetVariableData `json:"setVariableData" ocpp:"req,minItems=1"`
}

type SetVariableResult struct {
	AttributeType       string      `json:"attributeType,omitempty" ocpp:"enum=Actual|Target|MinSet|MaxSet"`
	AttributeStatus     string      `json:"attributeStatus" ocpp:"req,enum=Accepted|Rejected|UnknownComponent|UnknownVariable|NotSupportedAttributeType|RebootRequired"`
	AttributeStatusInfo *StatusInfo `json:"attributeStatusInfo,omitempty"`
	Component           Component   `json:"component" ocpp:"req"`
	Variable            Variable    `json:"variable" ocpp:"req"`
}

type SetVariables201Conf struct {
	SetVariableResult []SetVariableResult `json:"setVariableResult" ocpp:"req,minItems=1"`
}

type GetVariableData struct {
	AttributeType string    `json:"attributeType,omitempty" ocpp:"enum=Actual|Target|MinSet|MaxSet"`
	Component     Component `json:"component" ocpp:"req"`
	Variable      Variable  `json:"variable" ocpp:"req"`
}

type GetVariables201 struct {
	GetVariableData []GetVariableData `json:"getVariableData" ocpp:"req,minItems=1"`
}

type GetVariableResult struct {
	AttributeStatus     string      `json:"attributeStatus" ocpp:"req,enum=Accepted|Rejected|UnknownComponent|UnknownVariable|NotSupportedAttributeType"`
	AttributeStatusInfo *StatusInfo `json:"attributeStatusInfo,omitempty"`
	AttributeType       string      `json:"attributeType,omitempty" ocpp:"enum=Actual|Target|MinSet|MaxSet"`
	AttributeValue      string      `json:"attributeValue,omitempty" ocpp:"max=2500"`
	Component           Component   `json:"component" ocpp:"req"`
	Variable            Variable    `json:"variable" ocpp:"req"`
}

type GetVariables201Conf struct {
	GetVariableResult []GetVariableResult `json:"getVariableResult" ocpp:"req,minItems=1"`
}

type TriggerMessage201 struct {
	RequestedMessage string `json:"requestedMessage" ocpp:"req,enum=BootNotification|LogStatusNotification|FirmwareStatusNotification|Heartbeat|MeterValues|SignChargingStationCertificate|SignV2GCertificate|StatusNotification|TransactionEvent|SignCombinedCertificate|PublishFirmwareStatusNotification"`
	EVSE             *EVSE  `json:"evse,omitempty"`
}

type SetChargingProfile201 struct {
	EVSEID          int                `json:"evseId" ocpp:"req,min=0"`
	ChargingProfile ChargingProfile201 `json:"chargingProfile" ocpp:"req"`
}

type Firmware struct {
	Location           string `json:"location" ocpp:"req,max=512"`
	RetrieveDateTime   string `json:"retrieveDateTime" ocpp:"req,dt"`
	InstallDateTime    string `json:"installDateTime,omitempty" ocpp:"dt"`
	SigningCertificate string `json:"signingCertificate,omitempty" ocpp:"max=5500"`
	Signature          string `json:"signature,omitempty" ocpp:"max=800"`
}

type UpdateFirmware201 struct {
	Retries       *int     `json:"retries,omitempty"`
	RetryInterval *int     `json:"retryInterval,omitempty"`
	RequestID     int      `json:"requestId" ocpp:"req"`
	Firmware      Firmware `json:"firmware" ocpp:"req"`
}

type LogParameters struct {
	RemoteLocation  string `json:"remoteLocation" ocpp:"req,max=512"`
	OldestTimestamp string `json:"oldestTimestamp,omitempty" ocpp:"dt"`
	LatestTimestamp string `json:"latestTimestamp,omitempty" ocpp:"dt"`
}

type GetLog201 struct {
	Log           LogParameters `json:"log" ocpp:"req"`
	LogType       string        `json:"logType" ocpp:"req,enum=DiagnosticsLog|SecurityLog"`
	RequestID     int           `json:"requestId" ocpp:"req"`
	Retries       *int          `json:"retries,omitempty"`
	RetryInterval *int          `json:"retryInterval,omitempty"`
}

type GetLog201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected|AcceptedCanceled"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
	Filename   string      `json:"filename,omitempty" ocpp:"max=255"`
}

type UnlockConnector201 struct {
	EVSEID      int `json:"evseId" ocpp:"req"`
	ConnectorID int `json:"connectorId" ocpp:"req"`
}

// v201In are the actions a 2.0.1 charging station sends.
var v201In = map[string]msgDef{
	"BootNotification":           def[BootNotification201, BootNotification201Conf](),
	"Heartbeat":                  def[Empty, HeartbeatConf](),
	"StatusNotification":         def[StatusNotification201, Empty](),
	"Authorize":                  def[Authorize201, Authorize201Conf](),
	"TransactionEvent":           def[TransactionEvent201, TransactionEvent201Conf](),
	"MeterValues":                def[MeterValues201, Empty](),
	"NotifyEvent":                def[NotifyEvent201, Empty](),
	"SecurityEventNotification":  def[SecurityEventNotification201, Empty](),
	"FirmwareStatusNotification": def[FirmwareStatusNotification201, Empty](),
	"LogStatusNotification":      def[LogStatusNotification201, Empty](),
	"DataTransfer":               def[DataTransfer201, DataTransfer201Conf](),
}

// v201Out are the actions the CSMS sends.
var v201Out = map[string]msgDef{
	"RequestStartTransaction": def[RequestStartTransaction201, RequestStartTransaction201Conf]("Accepted"),
	"RequestStopTransaction":  def[RequestStopTransaction201, StatusConf201]("Accepted"),
	"Reset":                   def[Reset201, StatusConf201]("Accepted", "Scheduled"),
	"ChangeAvailability":      def[ChangeAvailability201, StatusConf201]("Accepted", "Scheduled"),
	"SetVariables":            def[SetVariables201, SetVariables201Conf](),
	"GetVariables":            def[GetVariables201, GetVariables201Conf](),
	"TriggerMessage":          def[TriggerMessage201, StatusConf201]("Accepted"),
	"SetChargingProfile":      def[SetChargingProfile201, StatusConf201]("Accepted"),
	"UpdateFirmware":          def[UpdateFirmware201, StatusConf201]("Accepted", "AcceptedCanceled"),
	"GetLog":                  def[GetLog201, GetLog201Conf]("Accepted", "AcceptedCanceled"),
	"UnlockConnector":         def[UnlockConnector201, StatusConf201]("Unlocked"),
	"ClearCache":              def[Empty, StatusConf201]("Accepted"),
	"DataTransfer":            def[DataTransfer201, DataTransfer201Conf]("Accepted"),
}
