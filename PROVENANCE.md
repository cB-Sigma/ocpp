# Provenance

## How the code was written

Written for SigmaM2M, a oneM2M platform, in Go from the published OCPP
documents below. No code was ported or copied from another
implementation. Message names, field names, enumerations, error codes and
subprotocol names are facts every implementation must use; the code
around them (the RPC connection, the validation and error mapping, the
Go types, the SOAP translation, the signed message wrapper, the leniency
helpers) is our own design. No text of the documents is reproduced;
comments cite clause numbers in our own words. An automated check found
no run of seven or more words shared between the module's comments or
strings and the descriptions in the OCA schema and appendix files.

The code came from SigmaM2M's OCPP adapter and moved to this module on
2026-10-06. Files that moved whole or nearly whole were moved with
`git mv`: `rpc.go`, `validate.go`, `schema.go`, `signed.go`, `soap.go`,
`subprotocol.go`, `lenient.go`, `timestamps.go`, `v16.go`, `v16sec.go`,
`v201.go`, `v201x.go`, `v21.go` and the tests `rpc_internal_test.go`,
`fuzz_internal_test.go`, `schema_test.go` and `schematree_test.go`. The
parts of those files that are CSMS behaviour (settings, warnings, the
SOAP listener and charger connection, time zone choice) stayed in
SigmaM2M; `decode.go` holds two functions cut out of the adapter. `oca.go`
and the example and device model tests are new. The copyright holder and
licence (Apache-2.0, Copyright Abhinav Gautam) were decided by the user.

Before the move, SigmaM2M generated Go tables from the OCA schema and
appendix files (the validation facts without descriptions). Those
generated files are not part of this module; it embeds the files
unmodified and reads them at run time instead (see NOTICE).

Where a document leaves a reading open or contradicts itself, the choice
made is recorded in SigmaM2M's register of specification defects, which
is not part of this module; comments refer to its entries by number (for
example F-1412: timestamps without a zone are read in a zone the
application chooses, `Rezone`) and say the choice where they make it.

On 2026-10-11 the simulator, its command and its test central system
moved out of this module into ocpp-sim (with `git mv`), so the library
does not depend on its simulator, and the module took its public path
`github.com/cB-Sigma/ocpp`.

On 2026-10-11 mobilityhouse/ocpp 2.1.0's charge point and
lorenzodonini/ocpp-go v0.19.0's 1.6 charge point and 2.0.1 charging
station were run against a central system built from this module alone
(`interop`). One problem was found and fixed: ocpp-go's 2.0.1 Reset
answer carries `"statusInfo": null`, which the schema refuses, and
`Deviations` counted that as a type error (never tolerable); it now reads
an optional property sent as null as absent and checks the message again
without it, so a required property sent as null still fails
(`TestDeviationsNullProperty`). `ValidateMessage`, `ValidateJSON` and
`DecodeIn` are unchanged and still refuse it.

For v0.2.0 (2026-10-11 and 12) the `csms` and `chargepoint` packages,
`check.go`, `pnc16.go`, the generator in `internal/gen21` and the `v21`
package were written new, in our own words, from the documents below
and the OCA 1.6 Plug & Charge application note. `v21/messages.go` is
generated from the embedded 2.1 schemas: only property names, types,
required lists and enumeration values are taken; no description is
copied. The roles were designed before seanlee0923/ocpp v0.3.0 was
read; that project was then read for features and run as an interop
peer only, and no code was taken from it.

## Sources

| Area (files) | Followed | Licence of the source | Taken |
|---|---|---|---|
| OCPP-J RPC framing, message types, error codes (rpc.go) | OCPP-J 1.6 and its errata; OCPP 2.0.1 part 4 and 2.1 part 4, with the 2026-06 errata | CC BY-ND 4.0 | facts only |
| 1.6 message types (v16.go, v16sec.go) | OCPP 1.6 edition 2; the OCA 1.6 security whitepaper edition 3 | CC BY-ND 4.0 | facts only |
| 2.0.1 and 2.1 message types and direction tables (v201.go, v201x.go, v21.go) | OCPP 2.0.1 edition 4 part 2; OCPP 2.1 edition 2 part 2 | CC BY-ND 4.0 | facts only |
| OCPP-S 1.6 (soap.go, soapwsdl.go) | OCPP-S 1.6 and its errata; the OCPP-S 1.6 WSDLs | CC BY-ND 4.0 | facts only |
| Signed messages (signed.go) | OCPP 2.x part 4 clause 7 | CC BY-ND 4.0 | facts only |
| Schemas and appendices (oca/) | OCPP 1.6 documentation package (2025-04-29); OCPP 2.0.1 part 3 schemas (2020-03-31) and appendices v1.5; OCPP 2.1 part 3 schemas (2025-01-23) and appendices v2.1 | CC BY-ND 4.0 | the files, unmodified |
| Plug & Charge over 1.6 (pnc16.go) | OCA application note "Using ISO 15118 Plug & Charge with OCPP 1.6" v1.0 (2020-09-16) | CC BY-ND 4.0 | facts only (message and field names, lengths, enumeration values, the vendorId) |
| Reconnection back-off (chargepoint) | OCPP 2.0.1 and 2.1 part 2, the OCPPCommCtrlr RetryBackOff variables | CC BY-ND 4.0 | facts only |
| 2.1 Go types (v21/messages.go) | the embedded OCPP 2.1 part 3 schemas | CC BY-ND 4.0 | names, types and enumerations, generated; no descriptions |
| Interop peer (interop/peer/cp.py) | our own script calling mobilityhouse/ocpp 2.1.0 (MIT), installed from PyPI at test time | MIT | nothing copied |

## Tests

| Test | Source |
|---|---|
| unit tests at the module root | written from the documents above; the schema tests check every embedded OCA file against its published digest |
| interop/ | mobilityhouse/ocpp 2.1.0's charge point (1.6, 2.0.1, 2.1) and ocpp-go v0.19.0's charge point (1.6) and charging station (2.0.1) against the `csms` package; `chargepoint` against ocpp-go's central system (1.6) and CSMS (2.0.1); seanlee0923/ocpp v0.3.0's station and CSMS (2.1) against `csms` and `chargepoint` |

## Other implementations

The comparison in README.md was made on 2026-10-11 from the source of
lorenzodonini/ocpp-go v0.19.0, mobilityhouse/ocpp 2.1.0 and SteVe 3.14.1,
and on 2026-10-12 of seanlee0923/ocpp v0.3.0,
read for features only. A line-by-line similarity check against them was
not done; no code was taken from them.
