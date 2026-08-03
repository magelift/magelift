# MageLift integrated-mode VCL (mount over /etc/varnish/default.vcl when baking
# a custom Varnish sidecar). Until then, the pinned official image uses its
# env-based default VCL, which already forwards /health to the Magento HTTP
# container — nginx/FrankenPHP must short-circuit /health with 200 OK.
#
# This file adds an explicit uncacheable pass for /health and a backend probe.

vcl 4.1;

import dynamic;
import std;

# Placeholder so `varnishd -C` accepts the VCL; runtime uses dynamic.director.
backend default none;

acl ipv4_only { "0.0.0.0"/0; }

sub vcl_init {
	new dynamic_director = dynamic.director(whitelist = ipv4_only);
}

sub vcl_recv {
	if (!(std.getenv("VARNISH_BACKEND_HOST") && std.getenv("VARNISH_BACKEND_PORT"))) {
		return (synth(503, "backend not configured"));
	}
	set req.backend_hint = dynamic_director.backend(std.getenv("VARNISH_BACKEND_HOST"), std.getenv("VARNISH_BACKEND_PORT"));
	if (std.getenv("VARNISH_BACKEND_PORT") == "80") {
		set req.http.host = std.getenv("VARNISH_BACKEND_HOST");
	} else {
		set req.http.host = std.getenv("VARNISH_BACKEND_HOST") + ":" + std.getenv("VARNISH_BACKEND_PORT");
	}
	if (req.url == "/health") {
		return (pass);
	}
}

sub vcl_backend_response {
	if (bereq.url == "/health") {
		set beresp.uncacheable = true;
		set beresp.ttl = 0s;
	}
}
