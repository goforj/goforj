# PhotoDrop Sample App Prompt

Build **PhotoDrop**, a private photo library for uploading, organizing, finding,
and sharing photos, in the stock GoForj project provided to you.

Do not modify the GoForj framework itself. Use the generated project's existing
components, maker commands, App composition points, and infrastructure
abstractions. If a requirement exposes a framework defect or missing capability,
document it instead of hiding it with copied framework code, direct backend
clients, or edits to framework-managed generated files.

## Product

PhotoDrop gives each user a private photo library. A user can upload photos,
organize them into albums, manually tag the people shown in them, search their
library, and share selected photos through a controlled link.

The core experience is:

1. Upload one or several photos.
2. Browse them in a responsive gallery and open a larger view.
3. Add photos to albums and tag people such as `Mum`, `Alex`, or `Sam`.
4. Search by person, album, date, title, or original filename.
5. Select photos and create a share link.
6. Optionally email the share link to a recipient.
7. Expire or revoke the link.
8. Move photos to trash and restore them before permanent deletion.

Libraries are private by default. Do not add a public feed, followers, likes,
comments, facial recognition, video, photo editing, or cloud-photo
synchronization.

## People And Sharing Model

A person tag is an owner-managed record and does not require that person to have
a PhotoDrop account. A person has a display name and an optional email address.
People are linked to photos manually; do not attempt automatic face detection.

A share contains a snapshot of explicitly selected photos. Adding another photo
to an album later must not silently add it to an existing share. A share has an
opaque token, an optional recipient email, an optional expiration time, and a
revoked state.

The public share view must expose only the selected photos and the minimal
metadata needed to display them. It must not reveal the owner's library,
internal IDs, storage keys, people records, or unrelated album contents.

## Data Model

Implement the smallest clear model that supports:

- photos with owner, title, optional description, captured date, original
  filename, content type, byte size, checksum, storage keys, processing state,
  trash state, and timestamps
- albums owned by a user
- people owned by a user
- photo-to-album membership
- photo-to-person tags
- shares and their selected photos
- append-only activity for important actions such as upload, tag, share,
  restore, and permanent deletion

Use migrations, constraints, repositories, and transactions according to the
generated project's conventions. Store image content in Storage rather than in
database columns.

## Upload And Processing

Accept JPEG and PNG images. Other formats may be supported when the generated
project already provides a suitable implementation, but they are not required.

For each accepted upload:

1. Validate content type, decoded image content, configured size limit, and
   non-empty dimensions.
2. Store the original through the generated Storage abstraction.
3. Persist its metadata.
4. Emit a photo-uploaded event.
5. Queue thumbnail generation.
6. Store the generated thumbnail through Storage and update processing state.

The request must not perform thumbnail generation synchronously. Retrying a
thumbnail job must be safe. A failure after storing an original but before
committing its metadata must not leave an untracked object permanently.

Keep image processing modest: one gallery thumbnail is sufficient. Preserve
the original rather than recompressing it as part of the request.

## Required Behavior

Implement authenticated workflows for:

1. Uploading one or multiple photos.
2. Browsing a paginated gallery with useful processing and failure states.
3. Viewing photo details and editing user-owned metadata.
4. Creating albums and adding or removing photos without duplicate membership.
5. Creating people and tagging or untagging them without duplicate tags.
6. Searching and filtering by person, album, captured-date range, title, and
   original filename.
7. Creating, viewing, emailing, revoking, and expiring a share.
8. Moving photos to trash, restoring them, and permanently deleting eligible
   trashed photos.

All authenticated library operations must be scoped to the current owner. Do
not rely on hidden UI controls as authorization.

## Framework Components

Exercise these components through their normal generated abstractions:

- **Auth:** protect private libraries and owner-only mutations.
- **Database:** persist metadata, organization, shares, and activity.
- **Storage:** store originals and thumbnails under non-guessable keys.
- **Queue and jobs:** generate thumbnails and deliver share emails
  asynchronously.
- **Events:** emit photo-uploaded, photo-tagged, share-created, share-revoked,
  and photo-deleted events. Use subscribers for secondary reactions where
  appropriate.
- **Mail:** send a concise optional share invitation through the configured
  development transport.
- **Scheduler:** expire shares and permanently purge photos that have remained
  in trash beyond a configurable retention period.
- **Cache:** cache gallery or album summary data and public share manifests.
  Invalidate affected entries so revocation and ownership changes are visible
  immediately.
- **CLI:** seed a visual demo library, import images from a directory, rebuild
  missing thumbnails, and report or purge eligible trash.
- **Metrics and Lighthouse:** preserve framework instrumentation and make the
  upload request, storage operations, thumbnail job, share email, and cleanup
  schedule understandable through existing observability surfaces.

Do not import a storage, cache, queue, mail, or event backend directly when the
generated application provides a manager or resource abstraction.

## HTTP And UI

Use the starter kit already selected in the project. Provide:

- sign-in
- paginated photo gallery
- multi-photo upload with clear per-file results
- photo detail or lightbox view
- albums view and album detail
- people view and person-filtered photos
- search and filters represented in the URL where appropriate
- share creation and management
- clean recipient-facing share view
- trash and restore view

Expose an authenticated JSON API for the same primary library workflows and the
minimal unauthenticated endpoint needed to resolve a valid public share. Follow
the project's routing and response conventions so route listing and API index
generation continue to work.

The UI should prioritize quick browsing and clear empty, loading, processing,
failure, and expired-share states. Do not build a large design system or an
image editor.

## Security And Consistency

- Generate share tokens with cryptographically secure randomness and store only
  what is needed to validate them safely.
- Never expose filesystem paths, backend object keys, or sequential database IDs
  as access credentials.
- Validate decoded content rather than trusting file extensions or multipart
  content types.
- Serve private image content only after owner or share authorization.
- Prevent path traversal when importing directories or deriving display names.
- Make share revocation effective even when a public view was cached.
- Keep database metadata and stored objects reconcilable after partial failure.

## Tests

Add focused tests proving at least:

- anonymous users cannot access private libraries
- one user cannot view, mutate, tag, share, or delete another user's photos
- invalid, disguised, empty, and oversized images are rejected
- failed metadata persistence does not permanently orphan a stored original
- thumbnail jobs are idempotent and expose a useful failure state
- album membership and people tags cannot be duplicated
- tagging, untagging, and album changes produce current search results
- a share exposes exactly its snapshotted photos
- guessed, expired, and revoked share tokens do not grant access
- revocation takes effect despite cached share data
- trash cleanup does not delete a restored photo
- concurrent or retried destructive operations remain safe

Use real generated integration boundaries where practical instead of replacing
the entire application with mocks. Use small generated image fixtures rather
than committing large binary test assets.

## Completion Criteria

The task is complete when:

- migrations apply to a fresh database
- the demo seed command creates a visually useful library and prints
  human-readable output
- upload, browse, organize, search, share, revoke, trash, and restore operate end
  to end
- thumbnails are generated asynchronously and can be rebuilt
- share mail can be observed through the configured development transport
- expiration and cleanup can be demonstrated without waiting for wall-clock
  time
- relevant backend and frontend tests pass
- `forj build` succeeds
- rerunning the project's normal generation/render workflow does not remove
  application behavior
- the final handoff lists commands run, test results, App-owned files changed,
  and any framework friction encountered

Keep the implementation deliberately focused. A fast, trustworthy photo
library is more valuable than a broad social product.
