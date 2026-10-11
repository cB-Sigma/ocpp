# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

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
