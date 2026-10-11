# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# A charge point on mobilityhouse/ocpp (MIT) for OCPP 1.6, 2.0.1 and 2.1,
# for the interop tests:
#
#   python cp.py <ws URL ending in the identity> <subprotocol> [password]
#
# It sends a fixed session (boot, heartbeat, authorize, a transaction with
# meter values), then one request that skips mobilityhouse's own schema
# check, prints "PEER waiting" and answers the central system's CALLs
# until the connection closes. Every step prints "PEER <event> <json>".

import asyncio
import base64
import dataclasses
import datetime
import json
import sys

import websockets

from ocpp.charge_point import camel_to_snake_case, snake_to_camel_case
from ocpp.exceptions import OCPPError
from ocpp.routing import on
from ocpp.v16 import ChargePoint as CP16
from ocpp.v16 import call as call16
from ocpp.v16 import call_result as result16
from ocpp.v201 import ChargePoint as CP201
from ocpp.v201 import call as call201
from ocpp.v201 import call_result as result201
from ocpp.v21 import ChargePoint as CP21
from ocpp.v21 import call as call21
from ocpp.v21 import call_result as result21


def emit(name, obj):
    print("PEER", name, json.dumps(obj, default=str), flush=True)


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def plain(obj):
    if dataclasses.is_dataclass(obj):
        obj = dataclasses.asdict(obj)
    if isinstance(obj, dict):
        return {k: plain(v) for k, v in obj.items() if v is not None}
    if isinstance(obj, list):
        return [plain(v) for v in obj]
    return obj


CONFIG = {"HeartbeatInterval": "300", "MeterValueSampleInterval": "60"}


def answer16(action, kw):
    if action == "GetConfiguration":
        asked = kw.get("key") or list(CONFIG)
        return dict(configuration_key=[{"key": k, "readonly": False, "value": CONFIG[k]} for k in asked if k in CONFIG],
                    unknown_key=[k for k in asked if k not in CONFIG] or None)
    if action == "ChangeConfiguration":
        CONFIG[kw["key"]] = kw["value"]
        return dict(status="Accepted")
    return dict(status="Accepted")


def answer2x(action, kw):
    if action == "GetVariables":
        out = []
        for d in kw["get_variable_data"]:
            v = CONFIG.get(d["variable"]["name"])
            r = {"component": d["component"], "variable": d["variable"],
                 "attribute_status": "Accepted" if v is not None else "UnknownVariable"}
            if v is not None:
                r["attribute_value"] = v
            out.append(r)
        return dict(get_variable_result=out)
    if action == "SetVariables":
        out = []
        for d in kw["set_variable_data"]:
            CONFIG[d["variable"]["name"]] = d["attribute_value"]
            out.append({"component": d["component"], "variable": d["variable"], "attribute_status": "Accepted"})
        return dict(set_variable_result=out)
    return dict(status="Accepted")


ACTIONS16 = ["RemoteStartTransaction", "RemoteStopTransaction", "Reset", "GetConfiguration",
             "ChangeConfiguration", "ClearCache"]
ACTIONS2X = ["RequestStartTransaction", "RequestStopTransaction", "Reset", "GetVariables",
             "SetVariables", "ClearCache"]


def handler(action, answer):
    async def h(self, **kw):
        emit("recv", {"action": action, "payload": snake_to_camel_case(kw)})
        return getattr(self.results, action)(**answer(action, kw))
    # mobilityhouse routes by the function's name, so each needs its own.
    h.__name__ = "on_" + action
    return on(action)(h)


def station(base, calls, results, actions, answer):
    attrs = {"calls": calls, "results": results}
    for a in actions:
        attrs["on_" + a] = handler(a, answer)
    return type("Station" + base.__module__.replace(".", "_"), (base,), attrs)


CLASSES = {
    "ocpp1.6": station(CP16, call16, result16, ACTIONS16, answer16),
    "ocpp2.0.1": station(CP201, call201, result201, ACTIONS2X, answer2x),
    "ocpp2.1": station(CP21, call21, result21, ACTIONS2X, answer2x),
}


def session16():
    yield "BootNotification", {"chargePointVendor": "PeerVendor", "chargePointModel": "PeerModel"}
    yield "Heartbeat", {}
    yield "StatusNotification", {"connectorId": 1, "status": "Available", "errorCode": "NoError"}
    yield "Authorize", {"idTag": "TAG-1"}
    tx = yield "StartTransaction", {"connectorId": 1, "idTag": "TAG-1", "meterStart": 1000, "timestamp": now()}
    txid = tx["transactionId"]
    yield "MeterValues", {"connectorId": 1, "transactionId": txid, "meterValue": [
        {"timestamp": now(), "sampledValue": [{"value": "1500", "measurand": "Energy.Active.Import.Register", "unit": "Wh"}]}]}
    yield "StopTransaction", {"transactionId": txid, "meterStop": 2000, "timestamp": now(), "reason": "Local"}


def session2x():
    yield "BootNotification", {"reason": "PowerUp", "chargingStation": {"vendorName": "PeerVendor", "model": "PeerModel"}}
    yield "Heartbeat", {}
    yield "StatusNotification", {"timestamp": now(), "connectorStatus": "Available", "evseId": 1, "connectorId": 1}
    yield "Authorize", {"idToken": {"idToken": "TAG-1", "type": "ISO14443"}}
    yield "TransactionEvent", {"eventType": "Started", "timestamp": now(), "triggerReason": "Authorized", "seqNo": 0,
                               "transactionInfo": {"transactionId": "PEER-TX-1"},
                               "idToken": {"idToken": "TAG-1", "type": "ISO14443"}, "evse": {"id": 1, "connectorId": 1}}
    yield "TransactionEvent", {"eventType": "Updated", "timestamp": now(), "triggerReason": "MeterValuePeriodic", "seqNo": 1,
                               "transactionInfo": {"transactionId": "PEER-TX-1", "chargingState": "Charging"},
                               "meterValue": [{"timestamp": now(), "sampledValue": [
                                   {"value": 1500, "measurand": "Energy.Active.Import.Register"}]}]}
    yield "TransactionEvent", {"eventType": "Ended", "timestamp": now(), "triggerReason": "StopAuthorized", "seqNo": 2,
                               "transactionInfo": {"transactionId": "PEER-TX-1", "stoppedReason": "Local"}}


async def send(cp, action, payload, raw=False):
    try:
        res = await cp.call(getattr(cp.calls, action)(**camel_to_snake_case(payload)), suppress=False,
                            skip_schema_validation=raw)
        out = snake_to_camel_case(plain(res))
        emit("result", {"action": action, "payload": out})
        return out
    except OCPPError as e:
        emit("error", {"action": action, "code": e.code, "description": str(e.description)})
    except Exception as e:  # noqa: BLE001 - reported to the test
        emit("error", {"action": action, "code": type(e).__name__, "description": str(e)})
    return None


async def main(url, proto, password=None):
    cp_id = url.rstrip("/").split("/")[-1]
    headers = {}
    if password:  # security profile 1: HTTP basic authentication
        headers["Authorization"] = "Basic " + base64.b64encode(f"{cp_id}:{password}".encode()).decode()
    async with websockets.connect(url, subprotocols=[proto], additional_headers=headers) as ws:
        emit("connected", {"id": cp_id, "subprotocol": ws.subprotocol})
        cp = CLASSES[proto](cp_id, ws)
        task = asyncio.create_task(cp.start())
        steps = session16() if proto == "ocpp1.6" else session2x()
        res = None
        while True:
            try:
                action, payload = steps.send(res)
            except StopIteration:
                break
            res = await send(cp, action, payload)
        # A request past mobilityhouse's own check: a string where the
        # schema wants an integer.
        if proto == "ocpp1.6":
            bad = {"connectorId": "one", "status": "Available", "errorCode": "NoError"}
        else:
            bad = {"timestamp": now(), "connectorStatus": "Available", "evseId": "one", "connectorId": 1}
        await send(cp, "StatusNotification", bad, raw=True)
        await send(cp, "Heartbeat", {})
        emit("waiting", {})
        try:
            await task
        except websockets.exceptions.ConnectionClosed:
            pass
        emit("closed", {})


asyncio.run(main(*sys.argv[1:4]))
