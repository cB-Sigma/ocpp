# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.2.0 (2026-10-12)

Both OCPP roles as library packages, typed 2.1 messages, Plug & Charge
over 1.6. Only additions; v0.1.0 code builds unchanged.

- `csms`: a central system role. One `http.Handler` for 1.6, 2.0.1 and
  2.1 at `<path>/<identity>`; an admission hook for security profiles 1
  to 3; handlers by action and version; every request and every answer
  of ours checked against the OCA schemas (1.6 security extension
  messages against the Go types); NotSupported for a known action with
  no handler, NotImplemented for an unknown one; a station registry with
  checked calls (`Station.Call`, `CallInto`, `Send`); a newer
  connection of the same identity replaces the older; the profile 3
  serial number check (`CheckSerial`); `OnInvalid` to tolerate
  deviations.
- `chargepoint`: a charging station role. `Dial` (identity escaped into
  the path, offered versions, basic authentication, TLS), `Client` with
  handlers, checked calls and SEND, and `Run`, which reconnects with the
  randomised doubling back-off of the 2.x RetryBackOff variables
  (`Backoff`).
- `v21`: Go types of every 2.1 request, response and SEND, generated
  from the embedded schemas (`go generate ./v21`); `Decode` and `Call`
  check payloads against the schemas. Closes the v0.1.0 "Not yet" row.
- `ValidateRequest`, `ValidateResponse` and `Known` for any version and
  either direction; `ActionOf(ctx)` gives a handler its action.
- ISO 15118 Plug & Charge over 1.6J after the OCA application note
  (2020): the nine messages as Go types and `PnC16Wrap`, `PnC16Unwrap`,
  `PnC16Answer`, `PnC16ReadAnswer` for the DataTransfer carriage.
- Interop: mobilityhouse/ocpp 2.1.0 and ocpp-go v0.19.0 now run against
  the `csms` package; new runs of `chargepoint` against ocpp-go's 1.6
  central system and 2.0.1 CSMS, and of both roles against
  seanlee0923/ocpp v0.3.0 in 2.1, SEND included. The interop module
  needs Go 1.26.5 (seanlee0923/ocpp's minimum).

## v0.1.0 (2026-10-11)

First release as a module of its own, split from SigmaM2M: OCPP-J RPC
for 1.6, 2.0.1 and 2.1 (`Conn`, with 2.1 SEND and CALLRESULTERROR),
validation against the embedded, unmodified OCA schemas, the 1.6 and
2.0.1 message types, OCPP-S 1.6, 2.x signed messages, subprotocol
negotiation, the leniency helpers and the device model appendices.

- `Deviations` reads an optional property sent as null as absent (a
  tolerable deviation) and checks the message again without it; a
  required property sent as null is still refused. ocpp-go's 2.0.1 Reset
  answer carries `"statusInfo": null`.
- The charge point simulator, its command and the test central system
  live in their own module,
  [ocpp-sim](https://github.com/cB-Sigma/ocpp-sim).
- Interop tests against mobilityhouse/ocpp 2.1.0 (in Docker) and
  lorenzodonini/ocpp-go v0.19.0 (in process) in a nested test-only
  module, `interop`.
