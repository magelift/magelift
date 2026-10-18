#!/usr/bin/env python3
"""Apply Magento-safe Cloud Armor request-body exclusions via Compute beta patchRule.

GA import stores GraphQL parsing and 64 KB inspection but drops
requestBodiesToExclude. Compute beta patchRule is the documented write path.
"""
from __future__ import annotations

import argparse
import json
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
from typing import Any, Dict, Optional

BETA = "https://compute.googleapis.com/compute/beta"


def access_token() -> str:
    for args in (
        ["gcloud", "auth", "application-default", "print-access-token"],
        ["gcloud", "auth", "print-access-token"],
    ):
        proc = subprocess.run(args, capture_output=True, text=True, check=False)
        token = proc.stdout.strip()
        if proc.returncode == 0 and token:
            return token
    raise SystemExit("gcloud did not return an access token")


def request_json(method: str, url: str, token: str, body: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    payload = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url,
        data=payload,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=60, context=ssl.create_default_context()) as resp:
            raw = resp.read()
    except urllib.error.HTTPError as err:
        detail = err.read().decode("utf-8", "replace")[:800]
        raise SystemExit(f"Cloud Armor beta {method} {url} failed HTTP {err.code}: {detail}") from err
    if not raw:
        return {}
    return json.loads(raw.decode())


def wait_operation(project: str, token: str, operation: Dict[str, Any]) -> None:
    name = str(operation.get("name") or "")
    if not name:
        raise SystemExit("Cloud Armor beta patchRule returned no operation name")
    if name.startswith("https://"):
        url = name.replace("/compute/v1/", "/compute/beta/")
    else:
        url = f"{BETA}/projects/{project}/global/operations/{name}"
    deadline = time.time() + 180
    while time.time() < deadline:
        got = request_json("GET", url, token)
        if got.get("status") == "DONE":
            if got.get("error"):
                raise SystemExit(f"Cloud Armor beta operation failed: {got['error']}")
            return
        time.sleep(2)
    raise SystemExit(f"Cloud Armor beta operation timed out: {name}")


def rule_needs_body_exclusion(rule: Dict[str, Any]) -> bool:
    for exclusion in ((rule.get("preconfiguredWafConfig") or {}).get("exclusions") or []):
        if exclusion.get("requestBodiesToExclude"):
            return True
    return False


def patchable_rule(rule: Dict[str, Any]) -> Dict[str, Any]:
    keep = ("description", "priority", "action", "preview", "match", "preconfiguredWafConfig", "rateLimitOptions")
    return {key: rule[key] for key in keep if key in rule}


def apply_body_exclusions(project: str, policy: str, want_path: str) -> None:
    token = access_token()
    want = json.loads(open(want_path, encoding="utf-8").read())
    want_rules = {int(rule["priority"]): rule for rule in want.get("rules") or [] if "priority" in rule}
    live = request_json("GET", f"{BETA}/projects/{project}/global/securityPolicies/{policy}", token)
    patched = 0
    for live_rule in live.get("rules") or []:
        priority = live_rule.get("priority")
        want_rule = want_rules.get(int(priority)) if priority is not None else None
        if want_rule is None or not rule_needs_body_exclusion(want_rule):
            continue
        body = patchable_rule(live_rule)
        body["preconfiguredWafConfig"] = want_rule["preconfiguredWafConfig"]
        op = request_json(
            "POST",
            f"{BETA}/projects/{project}/global/securityPolicies/{policy}/patchRule?priority={int(priority)}",
            token,
            body,
        )
        wait_operation(project, token, op)
        patched += 1
    if patched == 0:
        raise SystemExit("Cloud Armor beta patchRule found no Magento body-exclusion rules to patch")
    print(f"+ gcp-edge: patched Magento Cloud Armor body exclusions count={patched}", file=sys.stderr)


def export_policy(project: str, policy: str, dest: str) -> None:
    token = access_token()
    live = request_json("GET", f"{BETA}/projects/{project}/global/securityPolicies/{policy}", token)
    with open(dest, "w", encoding="utf-8") as handle:
        json.dump(live, handle)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("apply", "export"))
    parser.add_argument("--project", required=True)
    parser.add_argument("--policy", required=True)
    parser.add_argument("--want-json")
    parser.add_argument("--output")
    args = parser.parse_args()
    if args.action == "apply":
        if not args.want_json:
            raise SystemExit("apply requires --want-json")
        apply_body_exclusions(args.project, args.policy, args.want_json)
        return
    if not args.output:
        raise SystemExit("export requires --output")
    export_policy(args.project, args.policy, args.output)


if __name__ == "__main__":
    main()
