# ocpp

The protocol core of the Open Charge Point Protocol (OCPP 1.6J, OCPP-S 1.6, 2.0.1 and 2.1) in pure Go: RPC, schema validation in both directions, the SOAP binding, signed messages; not affiliated with the Open Charge Alliance.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/ocpp.svg)](https://pkg.go.dev/github.com/cB-Sigma/ocpp) [![test](https://github.com/cB-Sigma/ocpp/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/ocpp/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.26.0-00ADD8.svg)](go.mod)

One `Conn` speaks OCPP-J on either side of the connection, for every
version, including the 2.1 SEND and CALLRESULTERROR message kinds. Every
message can be checked, in both directions, against the Open Charge
Alliance's own JSON schemas, which this module embeds unmodified and
compiles when a version is first used; a failure comes back with the
CALLERROR code of that version. Around that: the OCPP-S 1.6 SOAP
binding, 2.x signed messages, the checks behind security profiles 1 to
3, the 2.0.1 and 2.1 device model appendices, and helpers for the
deviations real chargers send. Since v0.2.0 both roles are library
packages too: `csms` (a central system listener that admits, routes and
checks stations) and `chargepoint` (a charging station connection that
reconnects), with Go types for every 2.1 message in `v21`. It was
written for SigmaM2M, a oneM2M platform, whose charging station
management system (CSMS) is built on it; the CSMS's business logic
(charger state, authorization, tariffs, smart charging) is not part of
this module.

## Features

| Feature | Status |
|---|---|
| OCPP-J for 1.6, 2.0.1 and 2.1: CALL, CALLRESULT, CALLERROR, one outstanding CALL per direction, message IDs, time-outs, ping (`Conn`, `NewConn`) | yes |
| 2.1 SEND (`Conn.Send`) and CALLRESULTERROR (`Conn.CheckResult`, `OnResultError`) | yes |
| Unknown message types ignored (2.x errata 2026-06); unreadable message IDs answered with -1 (2.x) | yes |
| CALLs carried another way, as OCPP-S does (`NewSenderConn`) | yes |
| Validation of every 1.6, 2.0.1 and 2.1 request and response against the OCA schemas (`ValidateMessage`, `ValidateJSON`, `DecodeIn`, `CheckAnswer`); failures as `*RPCError` with the version's code (1.6's FormationViolation and OccurenceConstraintViolation spellings included) | yes |
| Go types for the 1.6 messages (with the security whitepaper's, which have no OCA schema) and the 2.0.1 messages (`V16In`, `V16Out`, `V201In`, `V201Out`); 2.1 direction tables (`In21`, `Out21`, `SendActions21`) | yes |
| Go types for every 2.1 request, response and SEND, generated from the embedded OCA schemas: enumerations as named string types with constants, optional properties as pointers, unknown CustomData properties kept (`v21`, `v21.Decode`, `v21.Call`) | yes (since v0.2.0) |
| Central system role: one `http.Handler` for 1.6, 2.0.1 and 2.1 at `<path>/<identity>`, an admission hook for security profiles 1 to 3, routing by action (and version), schema checks of every request and of our own answers, NotSupported and NotImplemented for unhandled actions, a registry of stations with typed calls, a newer connection replacing an older one, the profile 3 serial number check, a hook to tolerate deviations (`csms.New`, `Server.Handle`, `Station.Call`, `Config.OnInvalid`) | yes (since v0.2.0) |
| Charging station role: dial with the identity in the path, offered versions, basic authentication and TLS; routing of the central system's CALLs; schema checks both ways; 2.1 SEND; `Run` reconnects with the randomised doubling back-off of the 2.x RetryBackOff variables (`chargepoint.New`, `Dial`, `Client.Run`, `Backoff`) | yes (since v0.2.0) |
| `ValidateRequest` and `ValidateResponse` for any version and either direction; `ActionOf` gives a handler its action | yes (since v0.2.0) |
| ISO 15118 Plug & Charge over 1.6J as the OCA application note (2020) has it: the nine messages as Go types, wrapped in and read from DataTransfer with the note's vendorId (`PnC16`, `PnC16Wrap`, `PnC16Unwrap`, `PnC16Answer`, `PnC16ReadAnswer`) | yes (since v0.2.0) |
| OCPP-S 1.6: Central System requests in, Charge Point calls out, faults with OCPP sub-codes, gzip and deflate, elements in the WSDLs' order, envelope check against the WSDLs (`ReadSOAPRequest`, `WriteSOAPResponse`, `WriteSOAPFault`, `SOAPCall`, `CheckSOAPEnvelope`) | yes |
| 2.x signed messages, ES256, RS256 and RS384 (`MsgSigner`) | yes |
| Security profiles 1 to 3: the pieces a listener needs (subprotocol, TLS or plain WebSocket, the BootNotification serial number to compare with the client certificate), used by `csms` | yes; the password store and certificate trust are the application's |
| Subprotocol negotiation that takes variant spellings (`Subprotocols`, `VersionOf`, `OfferedSubprotocols`) | yes |
| Deviations real chargers send: zone-less timestamps (`Rezone`), vendor properties, lengths and enumeration values (`Deviations`, `DecodeLoose`, `EchoesOnly`); an optional property sent as null is read as absent (since v0.1.0) | yes |
| 2.0.1 and 2.1 device model appendices: components, variables, security event criticality (`DeviceModelOf`) | yes |
| OCPP 1.5 and earlier, OCPP-S 1.5 | no |
| WebSocket compression | the application's choice (`websocket.AcceptOptions`) |

## Choosing an OCPP library

Compared from each project's source at the version named, on
2026-10-11 (seanlee0923/ocpp on 2026-10-12). Check the projects
themselves before relying on a row.

| | this module | lorenzodonini/ocpp-go v0.19.0 | seanlee0923/ocpp v0.3.0 | mobilityhouse/ocpp 2.1.0 (Python) | SteVe 3.14.1 (Java) |
|---|---|---|---|---|---|
| What it is | protocol library: RPC, central system and charging station roles | protocol library: central system and charge point roles | protocol library: CSMS server and station client | protocol library, both sides | a CSMS application with a web UI and a database |
| OCPP-J 1.6 | yes, with the security extension | yes, with the security extension | yes | yes | yes, with the security extension |
| OCPP-S 1.6 (SOAP) | yes | no (its README rules it out) | no | no | yes, and 1.2 and 1.5 |
| OCPP 2.0.1 | yes | yes | yes | yes | no |
| OCPP 2.1 | yes, with SEND and CALLRESULTERROR | no | yes, with SEND (received by its CSMS; its station cannot send one); no CALLRESULTERROR | yes, without SEND and CALLRESULTERROR (message types 2 to 4 only) | no |
| Message validation | the OCA JSON schemas, embedded unmodified, both directions | validator struct tags written from the specification | Go validation generated from the OCA schemas | JSON schemas shipped with the package, both directions | JAXB types |
| Typed messages | 1.6, 2.0.1 and 2.1 (2.1 generated from the schemas) | 1.6 and 2.0.1 | 1.6, 2.0.1 and 2.1, generated | 1.6, 2.0.1 and 2.1 dataclasses | 1.2 to 1.6 |
| Reconnecting charging station | yes, the 2.x back-off variables' rule | yes | yes, exponential back-off | yes | n/a |
| 2.x signed messages | yes | no | no | no | n/a |
| ISO 15118 Plug & Charge over 1.6 (OCA application note) | yes | no | no | no | no |
| Charge point simulator | yes, every version, OCPP-S included, separate module | example programs | example programs | no | no |
| Interop evidence | against mobilityhouse/ocpp, ocpp-go and seanlee0923/ocpp, both roles (below) | none stated in the repository | none stated in the repository | none stated in the repository | its README reports an OCA 1.6 certification of a product built on it |
| Licence | Apache-2.0 (OCA files CC BY-ND 4.0) | MIT | MIT | MIT | GPL-3.0 |

Pick this module when you build a CSMS or a charger in Go and need 2.1
with SEND and CALLRESULTERROR, OCPP-S, Plug & Charge over 1.6, signed
messages, or the OCA schemas as the arbiter of what is valid, or a
simulator of every version for your tests. Pick ocpp-go when you need
typed 1.6 and 2.0.1 on both sides and do not need 2.1; it has the most
Go users. seanlee0923/ocpp is the closest Go alternative for 2.1 and has
more server operations tooling (metrics hooks, handshake rate limits,
graceful shutdown); pick it if those matter more than the rows above.
Pick mobilityhouse/ocpp in Python. Pick SteVe when you want a running
1.6 back office rather than a library.

## Install

```
go get github.com/cB-Sigma/ocpp@latest
```

Requires Go 1.26.0 or later.

## Quick start

```go
import "github.com/cB-Sigma/ocpp"

ws, _ := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: ocpp.Subprotocols([]string{ocpp.V21, ocpp.V201, ocpp.V16})})
version := ocpp.VersionOf(ws.Subprotocol())
c := ocpp.NewConn(ws, version, 30*time.Second, func(ctx context.Context, action string, p json.RawMessage) (any, error) {
	if _, verr := ocpp.ValidateJSON(version, action, false, p); verr != nil {
		return nil, verr // a CALLERROR with the version's code
	}
	return ocpp.CheckAnswer(version, action, answerFor(action), p)
})
go c.Serve(ctx, time.Minute)
res, err := c.Call(ctx, "GetVariables", req) // a *CallError when the charger refuses
```

This mirrors `ExampleNewConn` in example_test.go, which compiles with
the tests but does not run (it needs a listener). `ExampleValidateMessage`
and `ExampleDecodeIn` run and check their output.

With the role packages (v0.2.0), both sides are a few lines each:

```go
srv := csms.New(csms.Config{Authenticate: checkPassword})
srv.Handle("BootNotification", func(ctx context.Context, st *csms.Station, p json.RawMessage) (any, error) {
	return v21.BootNotificationResponse{CurrentTime: now(), Interval: 300, Status: v21.RegistrationStatusAccepted}, nil
})
http.Handle("/ocpp/", srv)

cp := chargepoint.New(chargepoint.Config{URL: "wss://csms.example/ocpp", ID: "CP1", Password: pw})
cp.Handle("Reset", onReset)
go cp.Run(ctx) // reconnects
res, err := v21.Call[v21.BootNotificationResponse](ctx, cp, v21.BootNotificationRequest{...})
```

`TestRolesEveryVersion` (csms/roles_test.go) and `TestTypedCall`
(v21/v21_test.go) run this.
[ocpp-sim](https://github.com/cB-Sigma/ocpp-sim) has a charge point of
every version to test a central system with.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/ocpp

| Package | What it does |
|---|---|
| `ocpp` (module root) | `Conn`, `NewConn`, `NewSenderConn`, `CallError`, `RPCError`, `ErrKind`, `ActionOf`; `ValidateMessage`, `ValidateJSON`, `ValidateRequest`, `ValidateResponse`, `Known`, `DecodeIn`, `CheckAnswer`, `HasSchema`, `LoadSchemas`, `OCAFiles`, `SchemaFiles`; the 1.6 and 2.0.1 message types and direction tables, the 2.1 direction tables; the OCPP-S 1.6 functions; `MsgSigner`; `Subprotocols`, `VersionOf`, `OfferedSubprotocols`; `Rezone`, `Deviations`, `DecodeLoose`, `EchoesOnly`; `DeviceModelOf`; the 1.6 Plug & Charge functions (`PnC16...`) |
| `csms` | the central system role: `New`, `Server` (`Handle`, `HandleVersion`, `Station`, `Stations`), `Station` (`Call`, `CallInto`, `Send`, `Close`, `Set`, `Get`), `Identity` |
| `chargepoint` | the charging station role: `New`, `Client` (`Connect`, `Run`, `Handle`, `Call`, `CallInto`, `Send`, `WaitConnected`), `Dial`, `EndpointURL`, `Backoff` |
| `v21` | generated Go types of every 2.1 message (`messages.go`, by `go generate ./v21` from the embedded schemas), `Messages`, `Decode`, `Call` |
| `oca/` (data) | the OCA files, unmodified: the 1.6 JSON schemas and OCPP-S WSDLs, the 2.0.1 and 2.1 JSON schemas and appendix CSV files, with each folder's attribution and SHA256SUMS |
| `interop` (nested module, tests only) | mobilityhouse/ocpp's charge point (in a container), ocpp-go's charge point and central systems and seanlee0923/ocpp's station and CSMS (in process) against the `csms` and `chargepoint` packages |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && go test -v ./...                          # ocpp-go and seanlee0923/ocpp, in process
cd interop && SIGMA_OCPP_INTEROP=1 go test -v ./...     # and mobilityhouse/ocpp, needs Docker
```

`TestOCASchemaFilesUnmodified` checks every embedded OCA file against
its published digest. Tests carry `// Covers: SIG-...` comments:
requirement IDs of the SigmaM2M project's compliance report, with no
meaning outside it; the tests added for the first release carry none.
Comments that name F-numbers (F-1412, say) refer to entries in that
project's register of specification defects; PROVENANCE.md says what
each choice is.

`TestEveryMessageRoundTrips` (v21) builds, for each of the 181 2.1
schemas, a payload with only the required properties and one with every
property, and checks that each decodes into the generated type and
encodes back to the same JSON, still valid; `TestGeneratedUpToDate`
fails when messages.go no longer matches the schemas.

`interop/` is a nested module with its own go.mod, so ocpp-go and
seanlee0923/ocpp stay out of this one; seanlee0923/ocpp v0.3.0 asks for
Go 1.26.5, so the interop module does too (the module itself stays on
1.26.0). Its mobilityhouse tests build an image from `interop/peer`
(python:3.12.10-slim with ocpp 2.1.0 and websockets 15.0.1 from PyPI)
and are skipped unless `SIGMA_OCPP_INTEROP=1` is set and Docker answers.

### Interop tested against

- mobilityhouse/ocpp 2.1.0 (2026-10-11, python:3.12.10-slim in Docker
  Desktop 29.4.3 on arm64; `interop`): its `ChargePoint` for 1.6, 2.0.1
  and 2.1 against a central system on this module (since v0.2.0 the
  `csms` package; 2026-10-11 again on it). Passed for every version:
  BootNotification, Heartbeat, StatusNotification, Authorize, a
  transaction (StartTransaction, MeterValues, StopTransaction in 1.6;
  TransactionEvent Started, Updated, Ended in 2.x), all accepted by our
  schema check; a StatusNotification with a string where the schema
  wants an integer, sent past mobilityhouse's own check, answered with
  TypeConstraintViolation, which mobilityhouse read as such; then
  RemoteStartTransaction, RemoteStopTransaction, Reset and
  GetConfiguration (1.6) or RequestStartTransaction,
  RequestStopTransaction, Reset, SetVariables and GetVariables (2.x) from
  our side, each answer valid against the OCA schema; a Reset outside
  the enumeration came back as a CALLERROR, and the connection carried on.
  mobilityhouse answers that with FormatViolation in 1.6 too, where
  OCPP-J 1.6 names the code FormationViolation; `Conn` takes any code.
- lorenzodonini/ocpp-go v0.19.0 (2026-10-11, in process; `interop`): its
  1.6 `ChargePoint` and 2.0.1 `ChargingStation` against the same central
  system, the same session and commands. Passed. ocpp-go's 2.0.1 Reset
  answer carries `"statusInfo": null`, which the 2.0.1 schema refuses;
  `Deviations` counted it as a type error until v0.1.0, which reads a
  null optional property as absent (`TestDeviationsNullProperty`).
  ocpp-go answers a Reset outside the enumeration with GenericError.
  Since v0.2.0 also the other role: this module's `chargepoint` (1.6 and
  2.0.1; offered 2.1 and 2.0.1, ocpp-go chose 2.0.1) against ocpp-go's
  1.6 central system and 2.0.1 CSMS: BootNotification, Heartbeat,
  StatusNotification and Authorize read by ocpp-go's handlers with the
  values sent, and a Reset from ocpp-go answered and read as Accepted.
- seanlee0923/ocpp v0.3.0 (2026-10-12, in process; `interop`), both
  roles in 2.1: its typed station booted and sent a heartbeat to the
  `csms` package, which checked both against the OCA schemas, and
  answered our Reset; our `chargepoint` booted against its CSMS, sent a
  2.1 SEND (NotifyPeriodicEventStream) that its typed SEND handler
  decoded, and answered its Reset. Passed. This is the first run of a
  SEND against another implementation. Its CSMS takes the whole request
  path as the identity, so the test mounts it under a prefix.
- The other way round for the simulator (their central systems against
  it) is in [ocpp-sim](https://github.com/cB-Sigma/ocpp-sim).
- SigmaM2M's CSMS, built on this module, against mobilityhouse/ocpp's
  charge point for all three versions (session, the refused request and
  commands from the platform), and the OCA part 6 CSMS test cases it
  runs against the simulator.
- Not yet: a real charger, the OCA's own test tool, OCPP-S against
  another implementation (SteVe speaks it; not run yet), CALLRESULTERROR
  and signed messages against another implementation (none of the
  peers above has them), Plug & Charge over 1.6 against another
  implementation (none of them has it).

## Not yet

| What | Why |
|---|---|
| OCPP 1.5 and OCPP-S 1.5 | the 1.5 documents are behind the OCA's download form; no user has asked |
| OCPP-S 1.6 against SteVe | SteVe is free (GPL-3.0) and speaks OCPP-S; the run needs its MySQL and Maven build in Docker and is planned for the next release |
| A run against real chargers and the OCA test tool | needs hardware and OCA membership |

## Related repositories

- [ocpp-sim](https://github.com/cB-Sigma/ocpp-sim): a simulated charge
  point of every version, as a Go package and a command.
- [jsonschema](https://github.com/cB-Sigma/jsonschema): the JSON Schema
  layer this module validates with, including the OCA's dialect.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability privately.

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)), except the
files in `oca/`, which are the Open Charge Alliance's, under CC BY-ND
4.0, unmodified. Copyright 2026 Abhinav Gautam. OCPP and Open Charge
Alliance are names of the Open Charge Alliance; they are used only to
say which protocol this module implements. This module is not made,
endorsed, certified or supported by the Open Charge Alliance.
