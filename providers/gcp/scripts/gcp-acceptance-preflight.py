#!/usr/bin/env python3
"""Verify the selected project and bounded create/cleanup IAM with effective ADC.

Tokens stay in process memory and HTTPS headers. Errors never echo response
bodies, command stderr, or credential values. No resources are mutated.
"""
import json
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def main():
    try:
        project, permissions = sys.argv[1], sys.argv[2:]
        if not permissions:
            raise ValueError("missing permission request")
        result = subprocess.run(
            ["gcloud", "auth", "application-default", "print-access-token"],
            capture_output=True, text=True, timeout=60, check=False,
        )
        token = result.stdout.strip()
        if result.returncode or not token or any(c.isspace() for c in token):
            print("GCP acceptance preflight: refreshable ADC is unavailable", file=sys.stderr)
            return 3
        base = "https://cloudresourcemanager.googleapis.com/v1/projects/" + urllib.parse.quote(project, safe="")
        opener = urllib.request.build_opener(NoRedirect)
        for endpoint, payload in [(base, None), (base + ":testIamPermissions", {"permissions": permissions})]:
            request = urllib.request.Request(
                endpoint, data=None if payload is None else json.dumps(payload).encode(),
                headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
            )
            with opener.open(request, timeout=30) as response:
                if response.status != 200:
                    print("GCP acceptance preflight: IAM/project HTTP status " + str(response.status), file=sys.stderr)
                    return 3
                body = json.load(response)
            if not isinstance(body, dict):
                raise ValueError("invalid response")
            if payload is None:
                if body.get("projectId") != project or body.get("lifecycleState") != "ACTIVE":
                    print("GCP acceptance preflight: selected project is inactive or differs from the requested target", file=sys.stderr)
                    return 2
            else:
                granted = body.get("permissions", [])
                if not isinstance(granted, list) or any(not isinstance(p, str) for p in granted):
                    raise ValueError("invalid permissions")
                missing = sorted(set(permissions) - set(granted))
                if missing:
                    print("GCP acceptance preflight: missing create or cleanup permission " + ", ".join(missing), file=sys.stderr)
                    return 2
    except urllib.error.HTTPError as error:
        print("GCP acceptance preflight: IAM/project HTTP status " + str(error.code), file=sys.stderr)
        return 3
    except Exception:
        print("GCP acceptance preflight: credential or IAM lookup did not complete", file=sys.stderr)
        return 3
    return 0


if __name__ == "__main__":
    sys.exit(main())
