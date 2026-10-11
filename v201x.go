package ocpp

// OCPP 2.0.1 message types for the device model (part 2 section B:
// GetBaseReport, GetReport, NotifyReport; section N: SetVariableMonitoring,
// ClearVariableMonitoring), local authorization lists (section D),
// reservations (section H), smart charging reads (section K:
// GetCompositeSchedule, ClearChargingProfile), certificate management
// (section M) and ISO 15118 support (sections C and M: Get15118EVCertificate,
// GetCertificateStatus). Field names, required properties, lengths and
// enumerations follow the 2.0.1 JSON schemas.

// ---- device model -------------------------------------------------------------

type ComponentVariable struct {
	Component Component `json:"component" ocpp:"req"`
	Variable  *Variable `json:"variable,omitempty"`
}

type GetBaseReport201 struct {
	RequestID  int    `json:"requestId" ocpp:"req"`
	ReportBase string `json:"reportBase" ocpp:"req,enum=ConfigurationInventory|FullInventory|SummaryInventory"`
}

type GetReport201 struct {
	RequestID         int                 `json:"requestId" ocpp:"req"`
	ComponentCriteria []string            `json:"componentCriteria,omitempty" ocpp:"minItems=1,maxItems=4,enum=Active|Available|Enabled|Problem"`
	ComponentVariable []ComponentVariable `json:"componentVariable,omitempty" ocpp:"minItems=1"`
}

type ReportStatusConf201 struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected|NotSupported|EmptyResultSet"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type VariableAttribute struct {
	Type       string  `json:"type,omitempty" ocpp:"enum=Actual|Target|MinSet|MaxSet"`
	Value      *string `json:"value,omitempty" ocpp:"max=2500"`
	Mutability string  `json:"mutability,omitempty" ocpp:"enum=ReadOnly|WriteOnly|ReadWrite"`
	Persistent *bool   `json:"persistent,omitempty"`
	Constant   *bool   `json:"constant,omitempty"`
}

type VariableCharacteristics struct {
	Unit               string   `json:"unit,omitempty" ocpp:"max=16"`
	DataType           string   `json:"dataType" ocpp:"req,enum=string|decimal|integer|dateTime|boolean|OptionList|SequenceList|MemberList"`
	MinLimit           *float64 `json:"minLimit,omitempty"`
	MaxLimit           *float64 `json:"maxLimit,omitempty"`
	ValuesList         string   `json:"valuesList,omitempty" ocpp:"max=1000"`
	SupportsMonitoring bool     `json:"supportsMonitoring" ocpp:"req"`
}

type ReportData struct {
	Component               Component                `json:"component" ocpp:"req"`
	Variable                Variable                 `json:"variable" ocpp:"req"`
	VariableAttribute       []VariableAttribute      `json:"variableAttribute" ocpp:"req,minItems=1,maxItems=4"`
	VariableCharacteristics *VariableCharacteristics `json:"variableCharacteristics,omitempty"`
}

type NotifyReport201 struct {
	RequestID   int          `json:"requestId" ocpp:"req"`
	GeneratedAt string       `json:"generatedAt" ocpp:"req,dt"`
	ReportData  []ReportData `json:"reportData,omitempty" ocpp:"minItems=1"`
	Tbc         bool         `json:"tbc,omitempty"`
	SeqNo       int          `json:"seqNo" ocpp:"req,min=0"`
}

type SetMonitoringData struct {
	ID          *int      `json:"id,omitempty"`
	Transaction *bool     `json:"transaction,omitempty"`
	Value       float64   `json:"value" ocpp:"req"`
	Type        string    `json:"type" ocpp:"req,enum=UpperThreshold|LowerThreshold|Delta|Periodic|PeriodicClockAligned"`
	Severity    int       `json:"severity" ocpp:"req,min=0"`
	Component   Component `json:"component" ocpp:"req"`
	Variable    Variable  `json:"variable" ocpp:"req"`
}

type SetVariableMonitoring201 struct {
	SetMonitoringData []SetMonitoringData `json:"setMonitoringData" ocpp:"req,minItems=1"`
}

type SetMonitoringResult struct {
	ID         *int        `json:"id,omitempty"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
	Status     string      `json:"status" ocpp:"req,enum=Accepted|UnknownComponent|UnknownVariable|UnsupportedMonitorType|Rejected|Duplicate"`
	Type       string      `json:"type" ocpp:"req,enum=UpperThreshold|LowerThreshold|Delta|Periodic|PeriodicClockAligned"`
	Component  Component   `json:"component" ocpp:"req"`
	Variable   Variable    `json:"variable" ocpp:"req"`
	Severity   int         `json:"severity" ocpp:"req"`
}

type SetVariableMonitoring201Conf struct {
	SetMonitoringResult []SetMonitoringResult `json:"setMonitoringResult" ocpp:"req,minItems=1"`
}

type ClearVariableMonitoring201 struct {
	ID []int `json:"id" ocpp:"req,minItems=1"`
}

type ClearMonitoringResult struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected|NotFound"`
	ID         int         `json:"id" ocpp:"req"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type ClearVariableMonitoring201Conf struct {
	ClearMonitoringResult []ClearMonitoringResult `json:"clearMonitoringResult" ocpp:"req,minItems=1"`
}

type GetMonitoringReport201 struct {
	RequestID          int                 `json:"requestId" ocpp:"req"`
	MonitoringCriteria []string            `json:"monitoringCriteria,omitempty" ocpp:"minItems=1,maxItems=3,enum=ThresholdMonitoring|DeltaMonitoring|PeriodicMonitoring"`
	ComponentVariable  []ComponentVariable `json:"componentVariable,omitempty" ocpp:"minItems=1"`
}

type VariableMonitoring struct {
	ID          int     `json:"id" ocpp:"req"`
	Transaction bool    `json:"transaction" ocpp:"req"`
	Value       float64 `json:"value" ocpp:"req"`
	Type        string  `json:"type" ocpp:"req,enum=UpperThreshold|LowerThreshold|Delta|Periodic|PeriodicClockAligned"`
	Severity    int     `json:"severity" ocpp:"req"`
}

type MonitoringData struct {
	Component          Component            `json:"component" ocpp:"req"`
	Variable           Variable             `json:"variable" ocpp:"req"`
	VariableMonitoring []VariableMonitoring `json:"variableMonitoring" ocpp:"req,minItems=1"`
}

type NotifyMonitoringReport201 struct {
	Monitor     []MonitoringData `json:"monitor,omitempty" ocpp:"minItems=1"`
	RequestID   int              `json:"requestId" ocpp:"req"`
	Tbc         bool             `json:"tbc,omitempty"`
	SeqNo       int              `json:"seqNo" ocpp:"req,min=0"`
	GeneratedAt string           `json:"generatedAt" ocpp:"req,dt"`
}

type SetMonitoringBase201 struct {
	MonitoringBase string `json:"monitoringBase" ocpp:"req,enum=All|FactoryDefault|HardWiredOnly"`
}

type SetMonitoringLevel201 struct {
	Severity int `json:"severity" ocpp:"req,min=0"`
}

// ---- charging profile reports -------------------------------------------------

type ChargingProfileCriterion struct {
	ChargingProfilePurpose string   `json:"chargingProfilePurpose,omitempty" ocpp:"enum=ChargingStationExternalConstraints|ChargingStationMaxProfile|TxDefaultProfile|TxProfile"`
	StackLevel             *int     `json:"stackLevel,omitempty"`
	ChargingProfileID      []int    `json:"chargingProfileId,omitempty" ocpp:"minItems=1"`
	ChargingLimitSource    []string `json:"chargingLimitSource,omitempty" ocpp:"minItems=1,maxItems=4,enum=EMS|Other|SO|CSO"`
}

type GetChargingProfiles201 struct {
	RequestID       int                      `json:"requestId" ocpp:"req"`
	EVSEID          *int                     `json:"evseId,omitempty"`
	ChargingProfile ChargingProfileCriterion `json:"chargingProfile" ocpp:"req"`
}

type GetChargingProfiles201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|NoProfiles"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type ReportChargingProfiles201 struct {
	RequestID           int                  `json:"requestId" ocpp:"req"`
	ChargingLimitSource string               `json:"chargingLimitSource" ocpp:"req,enum=EMS|Other|SO|CSO"`
	ChargingProfile     []ChargingProfile201 `json:"chargingProfile" ocpp:"req,minItems=1"`
	Tbc                 bool                 `json:"tbc,omitempty"`
	EVSEID              int                  `json:"evseId" ocpp:"req"`
}

// ---- local authorization list -------------------------------------------------

type AuthorizationData201 struct {
	IDToken     IDToken      `json:"idToken" ocpp:"req"`
	IDTokenInfo *IDTokenInfo `json:"idTokenInfo,omitempty"`
}

type SendLocalList201 struct {
	VersionNumber          int                    `json:"versionNumber" ocpp:"req"`
	UpdateType             string                 `json:"updateType" ocpp:"req,enum=Differential|Full"`
	LocalAuthorizationList []AuthorizationData201 `json:"localAuthorizationList,omitempty" ocpp:"minItems=1"`
}

type SendLocalList201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Failed|VersionMismatch"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type GetLocalListVersion201Conf struct {
	VersionNumber int `json:"versionNumber" ocpp:"req"`
}

// ---- reservations -------------------------------------------------------------

type ReserveNow201 struct {
	ID             int      `json:"id" ocpp:"req"`
	ExpiryDateTime string   `json:"expiryDateTime" ocpp:"req,dt"`
	ConnectorType  string   `json:"connectorType,omitempty" ocpp:"enum=cCCS1|cCCS2|cG105|cTesla|cType1|cType2|s309-1P-16A|s309-1P-32A|s309-3P-16A|s309-3P-32A|sBS1361|sCEE-7-7|sType2|sType3|Other1PhMax16A|Other1PhOver16A|Other3Ph|Pan|wInductive|wResonant|Undetermined|Unknown"`
	IDToken        IDToken  `json:"idToken" ocpp:"req"`
	EVSEID         *int     `json:"evseId,omitempty"`
	GroupIDToken   *IDToken `json:"groupIdToken,omitempty"`
}

type ReserveNow201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Faulted|Occupied|Rejected|Unavailable"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type CancelReservation201 struct {
	ReservationID int `json:"reservationId" ocpp:"req"`
}

type ReservationStatusUpdate201 struct {
	ReservationID           int    `json:"reservationId" ocpp:"req"`
	ReservationUpdateStatus string `json:"reservationUpdateStatus" ocpp:"req,enum=Expired|Removed"`
}

// ---- smart charging reads -----------------------------------------------------

type GetCompositeSchedule201 struct {
	Duration         int    `json:"duration" ocpp:"req"`
	ChargingRateUnit string `json:"chargingRateUnit,omitempty" ocpp:"enum=W|A"`
	EVSEID           int    `json:"evseId" ocpp:"req,min=0"`
}

type CompositeSchedule201 struct {
	ChargingSchedulePeriod []ChargingSchedulePeriod201 `json:"chargingSchedulePeriod" ocpp:"req,minItems=1"`
	EVSEID                 int                         `json:"evseId" ocpp:"req"`
	Duration               int                         `json:"duration" ocpp:"req"`
	ScheduleStart          string                      `json:"scheduleStart" ocpp:"req,dt"`
	ChargingRateUnit       string                      `json:"chargingRateUnit" ocpp:"req,enum=W|A"`
}

type GetCompositeSchedule201Conf struct {
	Status     string                `json:"status" ocpp:"req,enum=Accepted|Rejected"`
	StatusInfo *StatusInfo           `json:"statusInfo,omitempty"`
	Schedule   *CompositeSchedule201 `json:"schedule,omitempty"`
}

type ClearChargingProfileCriteria struct {
	EVSEID                 *int   `json:"evseId,omitempty"`
	ChargingProfilePurpose string `json:"chargingProfilePurpose,omitempty" ocpp:"enum=ChargingStationExternalConstraints|ChargingStationMaxProfile|TxDefaultProfile|TxProfile"`
	StackLevel             *int   `json:"stackLevel,omitempty"`
}

type ClearChargingProfile201 struct {
	ChargingProfileID       *int                          `json:"chargingProfileId,omitempty"`
	ChargingProfileCriteria *ClearChargingProfileCriteria `json:"chargingProfileCriteria,omitempty"`
}

// ---- certificates -------------------------------------------------------------

type CertificateHashData struct {
	HashAlgorithm  string `json:"hashAlgorithm" ocpp:"req,enum=SHA256|SHA384|SHA512"`
	IssuerNameHash string `json:"issuerNameHash" ocpp:"req,max=128"`
	IssuerKeyHash  string `json:"issuerKeyHash" ocpp:"req,max=128"`
	SerialNumber   string `json:"serialNumber" ocpp:"req,max=40"`
}

type CertificateHashDataChain struct {
	CertificateType          string                `json:"certificateType" ocpp:"req,enum=V2GRootCertificate|MORootCertificate|CSMSRootCertificate|V2GCertificateChain|ManufacturerRootCertificate"`
	CertificateHashData      CertificateHashData   `json:"certificateHashData" ocpp:"req"`
	ChildCertificateHashData []CertificateHashData `json:"childCertificateHashData,omitempty" ocpp:"minItems=1,maxItems=4"`
}

type InstallCertificate201 struct {
	CertificateType string `json:"certificateType" ocpp:"req,enum=V2GRootCertificate|MORootCertificate|CSMSRootCertificate|ManufacturerRootCertificate"`
	Certificate     string `json:"certificate" ocpp:"req,max=5500"`
}

type InstallCertificate201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected|Failed"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type GetInstalledCertificateIds201 struct {
	CertificateType []string `json:"certificateType,omitempty" ocpp:"minItems=1,enum=V2GRootCertificate|MORootCertificate|CSMSRootCertificate|V2GCertificateChain|ManufacturerRootCertificate"`
}

type GetInstalledCertificateIds201Conf struct {
	Status                   string                     `json:"status" ocpp:"req,enum=Accepted|NotFound"`
	StatusInfo               *StatusInfo                `json:"statusInfo,omitempty"`
	CertificateHashDataChain []CertificateHashDataChain `json:"certificateHashDataChain,omitempty" ocpp:"minItems=1"`
}

type DeleteCertificate201 struct {
	CertificateHashData CertificateHashData `json:"certificateHashData" ocpp:"req"`
}

type DeleteCertificate201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Failed|NotFound"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type SignCertificate201 struct {
	CSR             string `json:"csr" ocpp:"req,max=5500"`
	CertificateType string `json:"certificateType,omitempty" ocpp:"enum=ChargingStationCertificate|V2GCertificate"`
}

type SignCertificate201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type CertificateSigned201 struct {
	CertificateChain string `json:"certificateChain" ocpp:"req,max=10000"`
	CertificateType  string `json:"certificateType,omitempty" ocpp:"enum=ChargingStationCertificate|V2GCertificate"`
}

type CertificateSigned201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Rejected"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
}

type OCSPRequestData struct {
	HashAlgorithm  string `json:"hashAlgorithm" ocpp:"req,enum=SHA256|SHA384|SHA512"`
	IssuerNameHash string `json:"issuerNameHash" ocpp:"req,max=128"`
	IssuerKeyHash  string `json:"issuerKeyHash" ocpp:"req,max=128"`
	SerialNumber   string `json:"serialNumber" ocpp:"req,max=40"`
	ResponderURL   string `json:"responderURL" ocpp:"req,max=512"`
}

type Get15118EVCertificate201 struct {
	ISO15118SchemaVersion string `json:"iso15118SchemaVersion" ocpp:"req,max=50"`
	Action                string `json:"action" ocpp:"req,enum=Install|Update"`
	EXIRequest            string `json:"exiRequest" ocpp:"req,max=5600"`
}

type Get15118EVCertificate201Conf struct {
	Status      string      `json:"status" ocpp:"req,enum=Accepted|Failed"`
	StatusInfo  *StatusInfo `json:"statusInfo,omitempty"`
	EXIResponse string      `json:"exiResponse" ocpp:"req,max=5600"`
}

type GetCertificateStatus201 struct {
	OCSPRequestData OCSPRequestData `json:"ocspRequestData" ocpp:"req"`
}

type GetCertificateStatus201Conf struct {
	Status     string      `json:"status" ocpp:"req,enum=Accepted|Failed"`
	StatusInfo *StatusInfo `json:"statusInfo,omitempty"`
	OCSPResult string      `json:"ocspResult,omitempty" ocpp:"max=5500"`
}

func init() {
	for k, v := range map[string]msgDef{
		"NotifyReport":            def[NotifyReport201, Empty](),
		"NotifyMonitoringReport":  def[NotifyMonitoringReport201, Empty](),
		"ReportChargingProfiles":  def[ReportChargingProfiles201, Empty](),
		"ReservationStatusUpdate": def[ReservationStatusUpdate201, Empty](),
		"SignCertificate":         def[SignCertificate201, SignCertificate201Conf](),
		"Get15118EVCertificate":   def[Get15118EVCertificate201, Get15118EVCertificate201Conf](),
		"GetCertificateStatus":    def[GetCertificateStatus201, GetCertificateStatus201Conf](),
	} {
		v201In[k] = v
	}
	for k, v := range map[string]msgDef{
		"GetBaseReport":              def[GetBaseReport201, ReportStatusConf201]("Accepted"),
		"GetReport":                  def[GetReport201, ReportStatusConf201]("Accepted", "EmptyResultSet"),
		"GetMonitoringReport":        def[GetMonitoringReport201, ReportStatusConf201]("Accepted", "EmptyResultSet"),
		"SetMonitoringBase":          def[SetMonitoringBase201, ReportStatusConf201]("Accepted"),
		"SetMonitoringLevel":         def[SetMonitoringLevel201, StatusConf201]("Accepted"),
		"GetChargingProfiles":        def[GetChargingProfiles201, GetChargingProfiles201Conf]("Accepted", "NoProfiles"),
		"SetVariableMonitoring":      def[SetVariableMonitoring201, SetVariableMonitoring201Conf](),
		"ClearVariableMonitoring":    def[ClearVariableMonitoring201, ClearVariableMonitoring201Conf](),
		"SendLocalList":              def[SendLocalList201, SendLocalList201Conf]("Accepted"),
		"GetLocalListVersion":        def[Empty, GetLocalListVersion201Conf](),
		"ReserveNow":                 def[ReserveNow201, ReserveNow201Conf]("Accepted"),
		"CancelReservation":          def[CancelReservation201, StatusConf201]("Accepted"),
		"GetCompositeSchedule":       def[GetCompositeSchedule201, GetCompositeSchedule201Conf]("Accepted"),
		"ClearChargingProfile":       def[ClearChargingProfile201, StatusConf201]("Accepted"),
		"InstallCertificate":         def[InstallCertificate201, InstallCertificate201Conf]("Accepted"),
		"GetInstalledCertificateIds": def[GetInstalledCertificateIds201, GetInstalledCertificateIds201Conf]("Accepted", "NotFound"),
		"DeleteCertificate":          def[DeleteCertificate201, DeleteCertificate201Conf]("Accepted"),
		"CertificateSigned":          def[CertificateSigned201, CertificateSigned201Conf]("Accepted"),
	} {
		v201Out[k] = v
	}
}
