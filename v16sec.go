package ocpp

// OCPP 1.6J security extensions (OCA "Improved security for OCPP 1.6-J",
// edition 3): certificate management, security events, signed firmware
// updates and logs as first-class 1.6 actions (no DataTransfer). Field
// names, required properties, lengths and enumerations follow the
// whitepaper's JSON schemas; where a message is identical to its 2.0.1
// counterpart the 2.0.1 type is reused (without customData, which the 1.6
// validation refuses).

type InstallCertificate16 struct {
	CertificateType string `json:"certificateType" ocpp:"req,enum=CentralSystemRootCertificate|ManufacturerRootCertificate"`
	Certificate     string `json:"certificate" ocpp:"req,max=5500"`
}

type InstallCertificate16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Failed|Rejected"`
}

type GetInstalledCertificateIds16 struct {
	CertificateType string `json:"certificateType" ocpp:"req,enum=CentralSystemRootCertificate|ManufacturerRootCertificate"`
}

type GetInstalledCertificateIds16Conf struct {
	Status              string                `json:"status" ocpp:"req,enum=Accepted|NotFound"`
	CertificateHashData []CertificateHashData `json:"certificateHashData,omitempty" ocpp:"minItems=1"`
}

type DeleteCertificate16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Failed|NotFound"`
}

type SignCertificate16 struct {
	CSR string `json:"csr" ocpp:"req,max=5500"`
}

type SignCertificate16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Rejected"`
}

type CertificateSigned16 struct {
	CertificateChain string `json:"certificateChain" ocpp:"req,max=10000"`
}

type ExtendedTriggerMessage16 struct {
	RequestedMessage string `json:"requestedMessage" ocpp:"req,enum=BootNotification|LogStatusNotification|FirmwareStatusNotification|Heartbeat|MeterValues|SignChargePointCertificate|StatusNotification"`
	ConnectorID      *int   `json:"connectorId,omitempty" ocpp:"min=0"`
}

type ExtendedTriggerMessage16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Rejected|NotImplemented"`
}

type SignedUpdateFirmware16 struct {
	Retries       *int     `json:"retries,omitempty"`
	RetryInterval *int     `json:"retryInterval,omitempty"`
	RequestID     int      `json:"requestId" ocpp:"req"`
	Firmware      Firmware `json:"firmware" ocpp:"req"`
}

type SignedUpdateFirmware16Conf struct {
	Status string `json:"status" ocpp:"req,enum=Accepted|Rejected|AcceptedCanceled|InvalidCertificate|RevokedCertificate"`
}

type SignedFirmwareStatusNotification16 struct {
	Status    string `json:"status" ocpp:"req,enum=Downloaded|DownloadFailed|Downloading|DownloadScheduled|DownloadPaused|Idle|InstallationFailed|Installing|Installed|InstallRebooting|InstallScheduled|InstallVerificationFailed|InvalidSignature|SignatureVerified"`
	RequestID *int   `json:"requestId,omitempty"`
}

func init() {
	for k, v := range map[string]msgDef{
		"SignCertificate":                  def[SignCertificate16, SignCertificate16Conf](),
		"SecurityEventNotification":        def[SecurityEventNotification201, Empty](),
		"LogStatusNotification":            def[LogStatusNotification201, Empty](),
		"SignedFirmwareStatusNotification": def[SignedFirmwareStatusNotification16, Empty](),
	} {
		v16In[k] = v
	}
	for k, v := range map[string]msgDef{
		"InstallCertificate":         def[InstallCertificate16, InstallCertificate16Conf]("Accepted"),
		"GetInstalledCertificateIds": def[GetInstalledCertificateIds16, GetInstalledCertificateIds16Conf]("Accepted", "NotFound"),
		"DeleteCertificate":          def[DeleteCertificate201, DeleteCertificate16Conf]("Accepted"),
		"CertificateSigned":          def[CertificateSigned16, SignCertificate16Conf]("Accepted"),
		"ExtendedTriggerMessage":     def[ExtendedTriggerMessage16, ExtendedTriggerMessage16Conf]("Accepted"),
		"SignedUpdateFirmware":       def[SignedUpdateFirmware16, SignedUpdateFirmware16Conf]("Accepted"),
		"GetLog":                     def[GetLog201, GetLog201Conf]("Accepted"),
	} {
		v16Out[k] = v
	}
}
