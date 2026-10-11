package ocpp

import "reflect"

// OCPP 1.6 (edition 2) JSON message types: field names, required
// properties, string lengths and enumerations as the 1.6 JSON schemas
// define them.

type IDTagInfo16 struct {
	ExpiryDate  string `json:"expiryDate,omitempty" ocpp:"dt"`
	ParentIDTag string `json:"parentIdTag,omitempty" ocpp:"max=20"`
	Status      string `json:"status" ocpp:"req,enum=Accepted|Blocked|Expired|Invalid|ConcurrentTx"`
}

type SampledValue16 struct {
	Value     string `json:"value" ocpp:"req"`
	Context   string `json:"context,omitempty" ocpp:"enum=Interruption.Begin|Interruption.End|Sample.Clock|Sample.Periodic|Transaction.Begin|Transaction.End|Trigger|Other"`
	Format    string `json:"format,omitempty" ocpp:"enum=Raw|SignedData"`
	Measurand string `json:"measurand,omitempty" ocpp:"enum=Energy.Active.Export.Register|Energy.Active.Import.Register|Energy.Reactive.Export.Register|Energy.Reactive.Import.Register|Energy.Active.Export.Interval|Energy.Active.Import.Interval|Energy.Reactive.Export.Interval|Energy.Reactive.Import.Interval|Power.Active.Export|Power.Active.Import|Power.Offered|Power.Reactive.Export|Power.Reactive.Import|Power.Factor|Current.Import|Current.Export|Current.Offered|Voltage|Frequency|Temperature|SoC|RPM"`
	Phase     string `json:"phase,omitempty" ocpp:"enum=L1|L2|L3|N|L1-N|L2-N|L3-N|L1-L2|L2-L3|L3-L1"`
	Location  string `json:"location,omitempty" ocpp:"enum=Cable|EV|Inlet|Outlet|Body"`
	Unit      string `json:"unit,omitempty" ocpp:"enum=Wh|kWh|varh|kvarh|W|kW|VA|kVA|var|kvar|A|V|K|Celcius|Celsius|Fahrenheit|Percent"`
}

type MeterValue16 struct {
	Timestamp    string           `json:"timestamp" ocpp:"req,dt"`
	SampledValue []SampledValue16 `json:"sampledValue" ocpp:"req,minItems=1"`
}

type ChargingSchedulePeriod16 struct {
	StartPeriod  int     `json:"startPeriod" ocpp:"req"`
	Limit        float64 `json:"limit" ocpp:"req"`
	NumberPhases *int    `json:"numberPhases,omitempty"`
}

type ChargingSchedule16 struct {
	Duration               *int                       `json:"duration,omitempty"`
	StartSchedule          string                     `json:"startSchedule,omitempty" ocpp:"dt"`
	ChargingRateUnit       string                     `json:"chargingRateUnit" ocpp:"req,enum=A|W"`
	ChargingSchedulePeriod []ChargingSchedulePeriod16 `json:"chargingSchedulePeriod" ocpp:"req,minItems=1"`
	MinChargingRate        *float64                   `json:"minChargingRate,omitempty"`
}

type ChargingProfile16 struct {
	ChargingProfileID      int                `json:"chargingProfileId" ocpp:"req"`
	TransactionID          *int               `json:"transactionId,omitempty"`
	StackLevel             int                `json:"stackLevel" ocpp:"req,min=0"`
	ChargingProfilePurpose string             `json:"chargingProfilePurpose" ocpp:"req,enum=ChargePointMaxProfile|TxDefaultProfile|TxProfile"`
	ChargingProfileKind    string             `json:"chargingProfileKind" ocpp:"req,enum=Absolute|Recurring|Relative"`
	RecurrencyKind         string             `json:"recurrencyKind,omitempty" ocpp:"enum=Daily|Weekly"`
	ValidFrom              string             `json:"validFrom,omitempty" ocpp:"dt"`
	ValidTo                string             `json:"validTo,omitempty" ocpp:"dt"`
	ChargingSchedule       ChargingSchedule16 `json:"chargingSchedule" ocpp:"req"`
}

// ---- charge point → central system ----

type BootNotification16 struct {
	ChargePointVendor       string `json:"chargePointVendor" ocpp:"req,max=20"`
	ChargePointModel        string `json:"chargePointModel" ocpp:"req,max=20"`
	ChargePointSerialNumber string `json:"chargePointSerialNumber,omitempty" ocpp:"max=25"`
	ChargeBoxSerialNumber   string `json:"chargeBoxSerialNumber,omitempty" ocpp:"max=25"`
	FirmwareVersion         string `json:"firmwareVersion,omitempty" ocpp:"max=50"`
	ICCID                   string `json:"iccid,omitempty" ocpp:"max=20"`
	IMSI                    string `json:"imsi,omitempty" ocpp:"max=20"`
	MeterType               string `json:"meterType,omitempty" ocpp:"max=25"`
	MeterSerialNumber       string `json:"meterSerialNumber,omitempty" ocpp:"max=25"`
}

type BootNotification16Conf struct {
	Status      string `json:"status" ocpp:"req,enum=Accepted|Pending|Rejected"`
	CurrentTime string `json:"currentTime" ocpp:"req,dt"`
	Interval    int    `json:"interval" ocpp:"req"`
}

type Empty struct{}

type HeartbeatConf struct {
	CurrentTime string `json:"currentTime" ocpp:"req,dt"`
}

type StatusNotification16 struct {
	ConnectorID     int    `json:"connectorId" ocpp:"req,min=0"`
	ErrorCode       string `json:"errorCode" ocpp:"req,enum=ConnectorLockFailure|EVCommunicationError|GroundFailure|HighTemperature|InternalError|LocalListConflict|NoError|OtherError|OverCurrentFailure|PowerMeterFailure|PowerSwitchFailure|ReaderFailure|ResetFailure|UnderVoltage|OverVoltage|WeakSignal"`
	Info            string `json:"info,omitempty" ocpp:"max=50"`
	Status          string `json:"status" ocpp:"req,enum=Available|Preparing|Charging|SuspendedEVSE|SuspendedEV|Finishing|Reserved|Unavailable|Faulted"`
	Timestamp       string `json:"timestamp,omitempty" ocpp:"dt"`
	VendorID        string `json:"vendorId,omitempty" ocpp:"max=255"`
	VendorErrorCode string `json:"vendorErrorCode,omitempty" ocpp:"max=50"`
}

type Authorize16 struct {
	IDTag string `json:"idTag" ocpp:"req,max=20"`
}

type Authorize16Conf struct {
	IDTagInfo IDTagInfo16 `json:"idTagInfo" ocpp:"req"`
}

type StartTransaction16 struct {
	ConnectorID   int    `json:"connectorId" ocpp:"req,min=1"`
	IDTag         string `json:"idTag" ocpp:"req,max=20"`
	MeterStart    int    `json:"meterStart" ocpp:"req"`
	ReservationID *int   `json:"reservationId,omitempty"`
	Timestamp     string `json:"timestamp" ocpp:"req,dt"`
}

type StartTransaction16Conf struct {
	IDTagInfo     IDTagInfo16 `json:"idTagInfo" ocpp:"req"`
	TransactionID int32       `json:"transactionId" ocpp:"req"`
}

type StopTransaction16 struct {
	IDTag           string         `json:"idTag,omitempty" ocpp:"max=20"`
	MeterStop       int            `json:"meterStop" ocpp:"req"`
	Timestamp       string         `json:"timestamp" ocpp:"req,dt"`
	TransactionID   int32          `json:"transactionId" ocpp:"req"`
	Reason          string         `json:"reason,omitempty" ocpp:"enum=EmergencyStop|EVDisconnected|HardReset|Local|Other|PowerLoss|Reboot|Remote|SoftReset|UnlockCommand|DeAuthorized"`
	TransactionData []MeterValue16 `json:"transactionData,omitempty"`
}

type StopTransaction16Conf struct {
	IDTagInfo *IDTagInfo16 `json:"idTagInfo,omitempty"`
}

type MeterValues16 struct {
	ConnectorID   int            `json:"connectorId" ocpp:"req,min=0"`
	TransactionID *int32         `json:"transactionId,omitempty"`
	MeterValue    []MeterValue16 `json:"meterValue" ocpp:"req,minItems=1"`
}

type DataTransfer16 struct {
	VendorID  string `json:"vendorId" ocpp:"req,max=255"`
	MessageID string `json:"messageId,omitempty" ocpp:"max=50"`
	Data      string `json:"data,omitempty"`
}

type DataTransfer16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Rejected|UnknownMessageId|UnknownVendorId"`
	Data   string `json:"data,omitempty"`
}

type DiagnosticsStatusNotification16 struct {
	Status string `json:"status" ocpp:"req,enum=Idle|Uploaded|UploadFailed|Uploading"`
}

type FirmwareStatusNotification16 struct {
	Status string `json:"status" ocpp:"req,enum=Downloaded|DownloadFailed|Downloading|Idle|InstallationFailed|Installing|Installed"`
}

// ---- central system → charge point ----

type RemoteStartTransaction16 struct {
	ConnectorID     *int               `json:"connectorId,omitempty" ocpp:"min=1"`
	IDTag           string             `json:"idTag" ocpp:"req,max=20"`
	ChargingProfile *ChargingProfile16 `json:"chargingProfile,omitempty"`
}

type StatusConf16 struct {
	Status string `json:"status" ocpp:"req"`
}

type RemoteStopTransaction16 struct {
	TransactionID int32 `json:"transactionId" ocpp:"req"`
}

type Reset16 struct {
	Type string `json:"type" ocpp:"req,enum=Hard|Soft"`
}

type UnlockConnector16 struct {
	ConnectorID int `json:"connectorId" ocpp:"req,min=1"`
}

type ChangeAvailability16 struct {
	ConnectorID int    `json:"connectorId" ocpp:"req,min=0"`
	Type        string `json:"type" ocpp:"req,enum=Inoperative|Operative"`
}

type ChangeConfiguration16 struct {
	Key   string `json:"key" ocpp:"req,max=50"`
	Value string `json:"value" ocpp:"req,max=500"`
}

type GetConfiguration16 struct {
	Key []string `json:"key,omitempty" ocpp:"max=50"`
}

type KeyValue16 struct {
	Key      string  `json:"key" ocpp:"req,max=50"`
	Readonly bool    `json:"readonly" ocpp:"req"`
	Value    *string `json:"value,omitempty" ocpp:"max=500"`
}

type GetConfiguration16Conf struct {
	ConfigurationKey []KeyValue16 `json:"configurationKey,omitempty"`
	UnknownKey       []string     `json:"unknownKey,omitempty" ocpp:"max=50"`
}

type TriggerMessage16 struct {
	RequestedMessage string `json:"requestedMessage" ocpp:"req,enum=BootNotification|DiagnosticsStatusNotification|FirmwareStatusNotification|Heartbeat|MeterValues|StatusNotification"`
	ConnectorID      *int   `json:"connectorId,omitempty" ocpp:"min=0"`
}

type SetChargingProfile16 struct {
	ConnectorID        int               `json:"connectorId" ocpp:"req,min=0"`
	CSChargingProfiles ChargingProfile16 `json:"csChargingProfiles" ocpp:"req"`
}

type ClearChargingProfile16 struct {
	ID                     *int   `json:"id,omitempty"`
	ConnectorID            *int   `json:"connectorId,omitempty"`
	ChargingProfilePurpose string `json:"chargingProfilePurpose,omitempty" ocpp:"enum=ChargePointMaxProfile|TxDefaultProfile|TxProfile"`
	StackLevel             *int   `json:"stackLevel,omitempty"`
}

type GetCompositeSchedule16 struct {
	ConnectorID      int    `json:"connectorId" ocpp:"req,min=0"`
	Duration         int    `json:"duration" ocpp:"req,min=0"`
	ChargingRateUnit string `json:"chargingRateUnit,omitempty" ocpp:"enum=A|W"`
}

type GetCompositeSchedule16Conf struct {
	Status           string              `json:"status" ocpp:"req,enum=Accepted|Rejected"`
	ConnectorID      *int                `json:"connectorId,omitempty"`
	ScheduleStart    string              `json:"scheduleStart,omitempty" ocpp:"dt"`
	ChargingSchedule *ChargingSchedule16 `json:"chargingSchedule,omitempty"`
}

type UpdateFirmware16 struct {
	Location      string `json:"location" ocpp:"req"`
	Retries       *int   `json:"retries,omitempty"`
	RetrieveDate  string `json:"retrieveDate" ocpp:"req,dt"`
	RetryInterval *int   `json:"retryInterval,omitempty"`
}

type GetDiagnostics16 struct {
	Location      string `json:"location" ocpp:"req"`
	Retries       *int   `json:"retries,omitempty"`
	RetryInterval *int   `json:"retryInterval,omitempty"`
	StartTime     string `json:"startTime,omitempty" ocpp:"dt"`
	StopTime      string `json:"stopTime,omitempty" ocpp:"dt"`
}

type GetDiagnostics16Conf struct {
	FileName string `json:"fileName,omitempty" ocpp:"max=255"`
}

type ReserveNow16 struct {
	ConnectorID   int    `json:"connectorId" ocpp:"req,min=0"`
	ExpiryDate    string `json:"expiryDate" ocpp:"req,dt"`
	IDTag         string `json:"idTag" ocpp:"req,max=20"`
	ParentIDTag   string `json:"parentIdTag,omitempty" ocpp:"max=20"`
	ReservationID int    `json:"reservationId" ocpp:"req"`
}

type CancelReservation16 struct {
	ReservationID int `json:"reservationId" ocpp:"req"`
}

type AuthorizationData16 struct {
	IDTag     string       `json:"idTag" ocpp:"req,max=20"`
	IDTagInfo *IDTagInfo16 `json:"idTagInfo,omitempty"`
}

type SendLocalList16 struct {
	ListVersion            int                   `json:"listVersion" ocpp:"req"`
	LocalAuthorizationList []AuthorizationData16 `json:"localAuthorizationList,omitempty"`
	UpdateType             string                `json:"updateType" ocpp:"req,enum=Differential|Full"`
}

type GetLocalListVersion16Conf struct {
	ListVersion int `json:"listVersion" ocpp:"req"`
}

// MsgDef is one action: its request and confirmation types, and for
// CSMS-initiated actions the statuses that count as success (none: any
// valid answer succeeds).
type MsgDef struct {
	Req, Conf reflect.Type
	OK        []string
}

type msgDef = MsgDef

// Def is the MsgDef of an action with request type Req and confirmation
// type Conf.
func Def[Req, Conf any](ok ...string) MsgDef {
	return MsgDef{Req: reflect.TypeFor[Req](), Conf: reflect.TypeFor[Conf](), OK: ok}
}

func def[Req, Conf any](ok ...string) MsgDef { return Def[Req, Conf](ok...) }

// V16In, V16Out, V201In and V201Out are the actions of each version we
// have Go types for, by who sends them: In a charging station, Out the
// CSMS. The 1.6 tables include the 1.6 security extension messages.
var (
	V16In   = v16In
	V16Out  = v16Out
	V201In  = v201In
	V201Out = v201Out
)

// v16In are the actions a 1.6 charge point sends.
var v16In = map[string]msgDef{
	"BootNotification":              def[BootNotification16, BootNotification16Conf](),
	"Heartbeat":                     def[Empty, HeartbeatConf](),
	"StatusNotification":            def[StatusNotification16, Empty](),
	"Authorize":                     def[Authorize16, Authorize16Conf](),
	"StartTransaction":              def[StartTransaction16, StartTransaction16Conf](),
	"StopTransaction":               def[StopTransaction16, StopTransaction16Conf](),
	"MeterValues":                   def[MeterValues16, Empty](),
	"DataTransfer":                  def[DataTransfer16, DataTransfer16Conf](),
	"DiagnosticsStatusNotification": def[DiagnosticsStatusNotification16, Empty](),
	"FirmwareStatusNotification":    def[FirmwareStatusNotification16, Empty](),
}

// v16Out are the actions the central system sends.
var v16Out = map[string]msgDef{
	"RemoteStartTransaction": def[RemoteStartTransaction16, StatusConf16]("Accepted"),
	"RemoteStopTransaction":  def[RemoteStopTransaction16, StatusConf16]("Accepted"),
	"Reset":                  def[Reset16, StatusConf16]("Accepted"),
	"UnlockConnector":        def[UnlockConnector16, StatusConf16]("Unlocked"),
	"ChangeAvailability":     def[ChangeAvailability16, StatusConf16]("Accepted", "Scheduled"),
	"ChangeConfiguration":    def[ChangeConfiguration16, StatusConf16]("Accepted", "RebootRequired"),
	"GetConfiguration":       def[GetConfiguration16, GetConfiguration16Conf](),
	"TriggerMessage":         def[TriggerMessage16, StatusConf16]("Accepted"),
	"ClearCache":             def[Empty, StatusConf16]("Accepted"),
	"SetChargingProfile":     def[SetChargingProfile16, StatusConf16]("Accepted"),
	"ClearChargingProfile":   def[ClearChargingProfile16, StatusConf16]("Accepted"),
	"GetCompositeSchedule":   def[GetCompositeSchedule16, GetCompositeSchedule16Conf]("Accepted"),
	"UpdateFirmware":         def[UpdateFirmware16, Empty](),
	"GetDiagnostics":         def[GetDiagnostics16, GetDiagnostics16Conf](),
	"ReserveNow":             def[ReserveNow16, StatusConf16]("Accepted"),
	"CancelReservation":      def[CancelReservation16, StatusConf16]("Accepted"),
	"SendLocalList":          def[SendLocalList16, StatusConf16]("Accepted"),
	"GetLocalListVersion":    def[Empty, GetLocalListVersion16Conf](),
	"DataTransfer":           def[DataTransfer16, DataTransfer16Conf]("Accepted"),
}
