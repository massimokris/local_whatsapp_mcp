# Scoped engine implementation and verification

2026-10-23: Integrated engine implementation `856db01` through `cc5b7a9` and batched verified alias resolution `57a7967` through `945d284`. [The engine README](../../../vendor/wacli/README.md) records upstream revision `5578d1b6892f50a8f2329b11a6b6e27a1a81f88b`, the reproducible patch/build boundary, policy acknowledgement, and lifetime-pipe ownership.

Complete upstream suites passed both with and without `sqlite_fts5`; vet and focused policy race tests passed. Added fixtures cover policy failure before opening stores, initial/all/selected modes, verified aliases, content side tables/FTS, cached and queued media, phone retry, polls and cross-chat references, webhook suppression, catalog retention, and text/file rejection before transport. Real binary JSON calls were verified against a synthetic archive. No real WhatsApp account was linked; live linking/history/media recovery remains in [installation acceptance](../../acceptance.md).

Final review extends the alias response with explicit verified groups for native selection migration. Its integration evidence is appended in the final distribution resolution.

Final review repairs `7ad3140` were integrated through `c6de190`; the implementer reran native, backend race/vet, complete upstream/FTS5, real-engine boundary, and packaging regression checks. This task is closed; final installer acceptance is tracked separately in the distribution task.
