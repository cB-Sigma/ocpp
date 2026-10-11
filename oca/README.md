# Open Charge Alliance files (CC BY-ND 4.0)

`1.6/`, `2.0.1/` and `2.1/` are unmodified copies of files the Open Charge
Alliance publishes (<https://openchargealliance.org/protocols/open-charge-point-protocol/>):
the OCPP part 3 JSON schemas of each version, the OCPP-S 1.6 WSDLs, and the
OCPP 2.0.1 and 2.1 part 2 appendix CSV files. Each folder's `README.md` names
the published archive the files come from, and its `SHA256SUMS` lists their
digests.

Copyright © Open Charge Alliance. Licensed under the Creative Commons
Attribution-NoDerivatives 4.0 International Public License (CC BY-ND 4.0,
<https://creativecommons.org/licenses/by-nd/4.0/legalcode>). They are not under
this module's Apache-2.0 licence.

- The files are never changed. The package embeds them, compiles the schemas
  and parses the CSV files in memory when a version is first used. A
  correction the OCA publishes in an errata is applied in code.
- `TestOCASchemaFilesUnmodified` checks every file against `SHA256SUMS`.
  The digests were compared on 2026-10-11 with the files in the archives
  the OCA distributes; they are the same, byte for byte.
- In SigmaM2M, where this module was written, these folders are copies of
  that repository's specification data, and the test also checks that the
  copies are the same; outside it that part is skipped.
