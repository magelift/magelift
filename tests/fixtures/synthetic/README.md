# Synthetic fixtures (original, example-only)

Original synthetic shops for offline scenario tests. Every identity is
example-only (`*.example.test`, `*.example.invalid`, or a documented
placeholder); nothing is copied from private shops and no secrets appear
anywhere in this tree. The `synthetic` hygiene gate enforces this on every
run plus review.

Layout: `gcp-preview/` and `aws-preview/` hold recipe-shaped configs for
routing and validation coverage; `invalid/` holds one classified failure
each; `imports/` holds synthetic ACC and Upsun inputs for the importer.

These fixtures prove command routing, contracts, deterministic failures,
configuration handling, and offline local planning. They never prove a
store: no line here claims Magento installed, served assets, searched
products, or recovered a database. Store proof belongs to bounded live
acceptance (`reference-store-acceptance`), never to this suite.
