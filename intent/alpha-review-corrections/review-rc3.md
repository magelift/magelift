# Pre-rc.3 correction review

Reviewed 2026-09-17, against the preceding `review.md`.
Code reviewed and tested: `42e5de1483d87a6eb9fec4a99659fca74e062c8f`.
During review, Muse committed the rc.3 manifest changes as
`6b67359dd09516b22dc4133d3d042201c06c088c` and created the root rc.3 tag.
The latter commit changes only root/provider module manifests and sums; its
publication outcome is recorded separately below.

**Verdict: several previous findings are fixed, but the media implementation is
still unsafe and incomplete. Keep 3.1 and the acceptance gates open.**

The installer and cache corrections address the causes identified last time.
The image guard now has a useful integration test. Provider CI follows root Go
changes, skipped checks no longer satisfy publication, and module publication
has a real consumer check. Those improvements should stay.

The public-bucket change rests on an incorrect assumption about Magento's
remote storage. This is a release blocker for any environment containing real
store data. A dummy-store acceptance run would not establish data isolation.

**Remaining findings**

1. **P1: the public bucket exposes more than public product assets.**

   `providers/gcp/storage/storage.go:115-118` grants every anonymous caller
   `roles/storage.objectViewer` on the entire bucket. The new comments and R03
   spec assert that only public assets enter it and exclude paid downloads.
   Magento 2.4.9 does not implement that boundary: its remote filesystem routes
   both MEDIA and VAR_IMPORT_EXPORT to the configured driver. Import/export
   operations therefore share this bucket, regardless of its name or prefix.

   This is directly visible in the pinned upstream
   [RemoteStorage DI configuration](https://github.com/magento/magento2/blob/2.4.9/app/code/Magento/RemoteStorage/etc/di.xml),
   including the `customRemoteFilesystem` directory list and filesystem
   injection into import/report helpers. Adobe also documents remote
   [export storage](https://experienceleague.adobe.com/en/docs/commerce-admin/systems/data-transfer/data-export)
   and [import-history storage](https://experienceleague.adobe.com/en/docs/commerce-admin/systems/data-transfer/import/data-import).

   Import/export files may contain customer or commercial data. Excluding paid
   downloads does not address that exposure. A common prefix does not turn those
   objects into public assets. Keep the application bucket private and implement
   an explicit delivery boundary, or implement and verify actual separation of
   public assets from private application files. Do not fix this by changing only
   the prose. Acceptance needs a negative test: an unauthenticated request must
   fail for a private import/export fixture while a product image remains usable.

2. **P1: CLI media paths do not match Magento's configured remote-storage layout.**

   `providers/gcp/stack/component.go:149` passes `storage.MediaPrefix` (`media/`)
   as the remote-storage root prefix. `images/php-nginx/env.php:129` places it
   in `remote_storage/prefix`. Magento then adds the media directory URI below
   that root. Its default MEDIA URI is itself `media`.

   Consequently a Magento media-relative `catalog/product/a.jpg` maps to
   `media/media/catalog/product/a.jpg`, while
   `providers/gcp/plugin/media.go:277` imports a local media tree into
   `media/catalog/product/a.jpg`. Export strips only the outer prefix
   (`media.go:239-249`), producing an extra media directory and including other
   remote-storage directories rather than a clean pub/media tree. The advertised
   media URL in `storage.go:123-125` uses the same incomplete single-prefix model.

   This is a source-traced integration finding, not a claimed live upload result.
   The pinned upstream
   [DirectoryList defaults](https://github.com/magento/magento2/blob/2.4.9/lib/internal/Magento/Framework/App/Filesystem/DirectoryList.php),
   [remote filesystem](https://github.com/magento/magento2/blob/2.4.9/app/code/Magento/RemoteStorage/Filesystem.php),
   and [S3 driver factory](https://github.com/magento/magento2/blob/2.4.9/app/code/Magento/AwsS3/Driver/AwsS3Factory.php)
   establish the directory URI and independently applied adapter prefix.

   Define separate constants/contracts for the remote root and the Magento media
   subtree. Align Magento writes, CLI import/export, and delivery URLs. Add an
   integration test using Magento's real directory mapping: importing a normal
   pub/media tree must make its image readable by Magento; exporting it must
   recover the same relative paths and exclude private import/export files.
   The current fake-store tests assume the single-prefix layout and cannot prove it.

3. **P2: import containment still fails when a parent directory changes during traversal.**

   `providers/gcp/plugin/media.go:156-177` checks a leaf with Lstat, opens its
   absolute path, and compares file identity. Both operations follow parent
   directory symlinks. `filepath.WalkDir` can have already observed a real
   directory when that directory is replaced before traversal reaches it.

   I reproduced this deterministically through the actual `importMediaTree`:
   the fake store's first upload replaces a later directory with a symlink to an
   outside fixture directory. The importer then uploads the outside file and
   returns nil error. The fixture contains no actual secrets.

   Static file/directory symlink tests now pass, and export's `os.Root` use is
   an improvement. Apply root-relative containment to import opens too; a leaf
   identity comparison does not establish ancestry. Cover replacement between
   enumeration and traversal, not just a link present before WalkDir starts.
   The temporary regression test was `TestReviewImportParentSwap`; it failed
   with `outside file uploaded after parent-directory swap (err=<nil>)`.

4. **P2: a queued rerun still permits publication using an older success.**

   `scripts/release-checks-gate.sh:81` selects the greatest `(started_at, id)`.
   A newer queued run can have `started_at: null`, which sorts before an older
   completed success regardless of its higher ID. The general `active` check
   affects missing required names only; it does not correct that selection.

   A fixture with successful provider-verify ID 10 and queued provider-verify
   ID 11, with no start timestamp yet, prints `PASS provider-verify: success`
   and exits 0. The previous in-progress case is repaired, but the attempt has
   to be identified before comparing its state. Select attempts using a reliable
   run/attempt identity and test queued, waiting, in-progress, failed, and
   successful transitions. Keep skipped checks blocked.

5. **P2: literal onboarding leaves the shop dirty before the build.**

   The guide correctly asks the user to commit the initialized YAML. Later,
   `docs/onboarding.md:115-123` creates `magelift.providers.lock` in that shop,
   and then proceeds to build without telling the user to commit it.
   `internal/source/repository.go:46-51` rejects all untracked changes.
   On the described first-run path the new lock prevents the build.

   Add the explicit review/commit step after provider installation and before
   building. Preserve the clean-source requirement: the provider lock should be
   part of the reviewable project state. The source-download and working-directory
   corrections otherwise resolve the previous Dockerfile-location issue.

**Previous finding disposition**

| Prior finding | This review |
| --- | --- |
| 1: embedded version lacks v | Fixed in source; normalization and default-version command regression coverage added. |
| 2: media template rejected by image guard | Fixed in source; MAGENTO_DC markers and extracted Dockerfile guard test pass. PHP predicate not executed locally because PHP is absent. |
| 3: conditional allUsers binding invalid | Invalid condition removed, but replaced by the unsafe public-storage assumption in finding 1 above. Remains open. |
| 4: tagged module omits dependencies | rc.3 requirements are now committed before the root tag; real publication check added. Actual published consumer proof remains pending. |
| 5: media symlinks | Static links handled and dotted names preserved; parent replacement still escapes import containment. Partial. |
| 6: invalid lock overwritten | Fixed: bootstrap is limited to ErrNoLockfile, preserving existing-lock errors. |
| 7: cache lookup disagrees with install | Fixed for environment override and beside-CLI lock cache fallback; targeted tests pass. |
| 8: root edits skip provider CI | Fixed: provider job now follows the root Go change filter too. |
| 9: boundary claims contradict code | Substantially reconciled as an explicit deferral tied to second-provider work. Delete the remaining contradictory “Provider drops root-internal imports” bullet from R05's current requirements, or mark it deferred there too. |
| 10: onboarding expects missing source tree | Pinned source archive, registry setup and directory transitions are added. The new-lock commit step is still missing. |

**Release evidence and limitations**

The initial direct test command ran while Muse was editing requirements to rc.3.
It failed before tests because the root rc.3 module was not resolvable yet. I
then tested an isolated git-archive snapshot of `42e5de1`, preserving the shared
working tree and its release preparation. This distinguishes tested correction
code from release dependency availability.

The first rc.3 release run,
[35264891712](https://github.com/magelift/magelift/actions/runs/35264891712),
failed during GoReleaser on `6b67359`. Its log reports a checksum-service 404 /
unknown revision for the root rc.3 module. The new “Verify published modules”
step was skipped. This may be a tag/proxy propagation problem; it does not by
itself prove the corrected manifests are structurally wrong. A bounded wait for
module visibility before packaging, followed by real tagged consumer builds,
would produce useful evidence. Keep checksum verification enabled and tags
immutable. This report does not claim that a later retry has failed or passed.

Tests run with the authorized `GOMAXPROCS=8 GOFLAGS=-p=4 GOMEMLIMIT=4GiB`:

- Isolated snapshot: **526 tests passed across 8 packages**: providerhost, CLI,
  build/pipeline, GCP plugin/storage/runtime/stack, and distribution.
- The distribution tests compile a staged provider outside the workspace; they
  remain synthetic publication-shape proofs, separate from the real tag checks.
- Existing release-check gate shell suite: passed.
- Added parent-directory-swap probe: failed, reproducing finding 3.
- Added queued-check fixture: incorrectly passed, reproducing finding 4.
- Read-only rc.3 commit/tag/workflow inspection completed.
- No cloud mutations, image builds, or release actions performed. PHP remains
  unavailable locally. No source or existing intent edits made by this review.

Before closing media work, also prove the actual browser delivery path. The
current nginx config has no media-to-get.php route, and the GCP environment
wiring no longer passes MediaURL; a bucket output alone does not establish the
URL Magento emits or that nginx can retrieve an uncached remote image. Test a
fresh pod and a real Magento image URL, not only direct GCS curl.

GCS/S3 compatibility also needs actual driver evidence: Magento's S3 driver
requests private ACLs, while this bucket enables uniform bucket-level access.
Google documents rejection of ACL operations under that setting. I have not
executed that exact upload against GCS, so record it as a required compatibility
check rather than an observed cloud failure.
[Magento driver](https://github.com/magento/magento2/blob/2.4.9/app/code/Magento/AwsS3/Driver/AwsS3.php),
[Google uniform access behavior](https://docs.cloud.google.com/storage/docs/uniform-bucket-level-access).

**Next step**

Resolve the media storage/delivery boundary and path contract before the live
proof. Complete the two regression fixes and onboarding commit instruction,
then verify the exact candidate's published modules and packaged install flow.
Keep 1.4, 2.5 and 5.1 open until their evidence exists. The independent-provider
refactor can remain explicitly deferred; privacy and working media cannot.
