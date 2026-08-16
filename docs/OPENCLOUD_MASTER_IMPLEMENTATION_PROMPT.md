> **Implementation status — 16 August 2026:** This is the user's original OpenCloud master implementation prompt, preserved below. Only this status section and the Slice headings have been annotated. “Complete” means the documented slice is implemented and verified; it does **not** mean that deferred production adapters or public deployment controls are enabled.
>
> | Slice | Status | Delivered scope |
> | --- | --- | --- |
> | 1 — Project Domain Model | ✅ Complete | Tenant-scoped projects, services, deployments/events, compatibility with existing sites, APIs, and dashboard screens. |
> | 2 — Build Abstraction | ✅ Complete | Build-provider contracts plus static, Railpack-planning, PHP, and fake providers; unsafe builds remain disabled. |
> | 3 — Isolated Builder | ✅ Complete (safe foundation) | Builder contracts, state/log abstractions, BuildKit boundary, resource-limit model, and fakes; no production build executor is enabled. |
> | 4 — Registry + Deployments | ✅ Complete (safe foundation) | Digest-only registry/revision model, lifecycle protection, health-before-traffic-switch workflow, rollback, and PostgreSQL integration coverage; no live registry/Docker/Caddy adapter or public deployment endpoint is enabled. |
> | 5 — Logs | ✅ Complete (safe foundation) | Tenant-safe Loki contract, collector configuration, Go query/SSE APIs, authenticated BFF, and Logs Viewer; production emitters remain disabled. |
> | 6 — Env/Secrets | ✅ Complete | Tenant/service/environment-scoped variables and AES-256-GCM encrypted secrets with rotation, boundary redaction, reveal auditing; authenticated BFF routes and the per-service Environment Variables manager UI. |
> | 7 — Database Manager | ✅ Complete (safe foundation) | Short-lived database console sessions, audited SQL Console execution, phpMyAdmin redirect for MariaDB, and dashboard manager UI; no public `db.<platform-domain>` gateway is deployed. |
> | 8 — Object Storage | ✅ Complete (safe foundation) | Bucket/object models with quotas, S3-compatible provider (plus fake), presigned GET/PUT, async storage jobs, bucket manager and object browser UI; production storage backend is opt-in via configuration. |
>
> Beyond the slices: Git source fields on services with a GitHub webhook route, preview-deployment records wired into build job handlers, persistent-storage quotas on services, and a resource-usage metering table. The end-to-end deployment path (source acquisition → build execution → registry transport → runtime rollout) remains intentionally disabled per its fail-closed design.
>
> For implementation details and validation evidence, see [ROADMAP.md](../ROADMAP.md), [docs/BACKEND.md](BACKEND.md), and [CHANGELOG.md](../CHANGELOG.md).

---

# OPENCLOUD — MASTER IMPLEMENTATION PROMPT

You are the principal engineer responsible for evolving the existing repository:

Nafixhutao/opencloud

into a production-oriented universal application hosting platform.

IMPORTANT:
This is an EXISTING repository.
Do NOT rewrite the project from scratch.
Do NOT replace the existing architecture without a strong technical reason.
Do NOT blindly copy Railway, Vercel, Supabase, Coolify, Dokploy, or other platforms.

The desired product is OpenCloud.

Railway/Vercel/Supabase are UX and product references only.

============================================================
0. FIRST: INSPECT THE REPOSITORY
============================================================

Before changing any code:

Read and understand at minimum:

- CLAUDE.md
- README.md
- ROADMAP.md
- ARCHITECTURE.md
- docs/BACKEND.md
- docs/DATABASE.md
- docs/HOSTING.md
- docs/SECURITY.md
- docs/INFRASTRUCTURE.md
- docs/DEPLOYMENT.md
- docs/API.md
- docs/TESTING.md
- relevant ADRs
- frontend dashboard structure
- backend migrations
- provisioner interfaces
- queue/worker implementation
- database provisioning code
- Caddy/Docker code

Treat repository documentation and CLAUDE.md rules as mandatory.

CRITICAL REPOSITORY RULE:
Never commit, push, merge, or publish changes without explicit human approval.

You may edit the working tree, run tests, create migrations, and prepare changes,
but stop before commit/push.

Never edit an already shipped migration.
Add new forward migrations.

============================================================
1. EXISTING OPENCLOUD FOUNDATION
============================================================

Preserve the existing architecture.

Current direction includes:

Frontend:
- Next.js
- TypeScript
- existing UI architecture
- existing authentication/BFF architecture

Backend:
- Go
- Gin
- Bun ORM
- PostgreSQL as control-plane system of record
- Redis for cache/session/rate limiting
- PostgreSQL-backed async job queue
- Zap structured logging
- Docker runtime backend
- Caddy ingress/domain/TLS
- provider-neutral provisioner interfaces

Existing security principles MUST remain:

- account_id tenant isolation on every customer-owned resource
- customer resources never cross tenant boundaries
- only authorized backend/provisioner layers may access Docker/Caddy
- public API/dashboard must never receive Docker socket access
- no secrets in logs
- no passwords in audit metadata
- no customer credentials in job payloads
- no root database credentials exposed to users
- customer database credentials use least privilege
- append-only audit trail
- HTTPS everywhere
- production database transport uses verified TLS
- containers are non-root where possible
- drop Linux capabilities
- no-new-privileges
- CPU/RAM/PID limits
- no arbitrary host mounts
- runtime resources are quota-controlled
- arbitrary Dockerfile builds must NOT happen on the production runtime worker

Current `sites` functionality should continue working while the new project/service
model is introduced gradually.

Do not break current Phase 0–3 functionality.

============================================================
2. PRODUCT VISION
============================================================

OpenCloud should become a universal application platform similar in developer
experience to Vercel/Railway, while also supporting traditional hosting needs
such as PHP, MariaDB, file storage, SQL management, and database administration.

The user experience should eventually be:

Connect GitHub or upload source
        ↓
OpenCloud detects the application
        ↓
Detect monorepo/services
        ↓
Build securely
        ↓
Create OCI image
        ↓
Push to private registry
        ↓
Security checks
        ↓
Deploy immutable revision
        ↓
Health check
        ↓
Switch Caddy traffic
        ↓
HTTPS domain
        ↓
Live logs
        ↓
Database / Storage / Environment / Metrics

A user must NOT normally need to manually select a programming language.

Support should be architecture-driven, not based on hardcoded language-specific
business logic.

Target ecosystems include, but are not limited to:

- HTML/CSS/JavaScript static sites
- React
- Vite
- Next.js
- Node.js
- TypeScript
- Go
- PHP
- Laravel
- Symfony
- WordPress later
- Python
- FastAPI
- Django
- Java
- Spring Boot
- Rust
- Ruby
- .NET
- Elixir
- other ecosystems supported by the build provider

============================================================
3. NEW CORE DOMAIN MODEL
============================================================

Introduce the concept:

Account
└── Project
    ├── Environment
    ├── Service(s)
    │   └── Deployment revisions
    ├── Database(s)
    ├── Storage bucket(s)
    └── Routes/domains

A project may contain multiple services.

Example:

Project: toko-online

frontend
- Next.js
- web service

backend
- Go
- API service

worker
- Python
- background worker

postgres
- managed database

redis
- cache

bucket
- object storage

This is essential for monorepos.

A React + Go repository must NOT be forced into a single container.

It should become:

Project
├── frontend service
└── backend service

Create/add schemas equivalent to:

projects
services
deployments
deployment_events
source_artifacts
service_routes
environment_variables / secrets metadata

Use the naming and migration conventions already defined by OpenCloud.

All customer-owned rows MUST contain account_id or otherwise have a structurally
safe tenant relationship.

Suggested service types:

- web
- worker
- cron
- static

Do not delete the existing `sites` table immediately.

Create a migration/compatibility strategy.

============================================================
4. SOURCE ACQUISITION
============================================================

OpenCloud should eventually accept:

- GitHub repository
- ZIP upload
- static files
- Dockerfile
- templates
- Docker images only under explicit image policy
- private Git repositories later

Initial implementation priority:

1. Git repository source
2. ZIP/source upload
3. static application
4. Dockerfile fallback after isolated build infrastructure exists

Source artifacts must have:

- immutable deployment identity
- commit SHA when Git-based
- source hash
- size limit
- safe archive extraction
- zip-slip prevention
- symlink validation
- archive bomb protection
- cleanup after build

============================================================
5. BUILD ARCHITECTURE
============================================================

Create a generic build abstraction.

For example:

type BuildProvider interface {
    Detect(...)
    Plan(...)
    Build(...)
}

Implement providers conceptually like:

- FakeBuildProvider
- StaticBuildProvider
- RailpackBuildProvider
- DockerfileBuildProvider

Use Railpack as the default automatic build engine.

Use BuildKit as the build execution engine.

DO NOT implement a language switch like:

if language == "go"
if language == "php"
if language == "python"

The build provider should detect and create the plan.

Dockerfile support MUST only be enabled through an isolated builder.

============================================================
6. ISOLATED BUILD WORKER
============================================================

Untrusted user source code must NEVER build inside:

- public API
- Next.js dashboard
- control-plane database host
- production container runtime with unrestricted Docker socket
- a worker holding production credentials

Architecture:

OpenCloud API
    ↓
PostgreSQL job queue
    ↓
isolated builder
    ↓
BuildKit
    ↓
OCI artifact
    ↓
private registry

The builder requires:

- rootless execution where practical
- CPU limits
- RAM limits
- PID limits
- source size limit
- image size limit
- build timeout
- ephemeral filesystem
- automatic cleanup
- network policy
- no production secrets
- no control-plane DB credential
- no unrestricted runtime-node Docker access
- deterministic labels/metadata
- build cancellation support

Build state lifecycle should support something close to:

queued
cloning
detecting
planning
building
pushing
scanning
deploying
ready
failed
cancelled

============================================================
7. OCI REGISTRY
============================================================

Add a RegistryProvider abstraction.

Initial backend may use CNCF Distribution.

Do not tightly couple the product to one registry.

Conceptual interface:

RegistryProvider
- Push
- Delete
- Exists
- ResolveDigest

Every successful deployment should reference immutable OCI digest.

Never depend on `latest`.

Example identity:

registry.internal/opencloud/<account>/<project>/<service>@sha256:...

Persist:

- digest
- image size
- build metadata
- build provider
- source revision
- creation timestamp

Harbor is a possible future upgrade, not mandatory for initial implementation.

============================================================
8. IMMUTABLE DEPLOYMENTS AND ROLLBACK
============================================================

Every deploy creates a new Deployment revision.

Example:

deployment v17
deployment v18
deployment v19 ← active

Deployment sequence:

build
→ push
→ optionally scan
→ start revision
→ readiness/health check
→ switch Caddy route
→ mark active
→ gracefully retire old revision

Old revision should remain available according to retention policy.

Rollback:

select previous deployment
→ start/verify if required
→ health check
→ switch traffic
→ mark active

Do NOT mutate the old image.

Design this so zero/low-downtime traffic switching can be supported.

============================================================
9. MONOREPO SUPPORT
============================================================

OpenCloud must support repositories like:

apps/
  web/
  admin/
  api/
workers/
  email/

The platform should be able to discover candidate applications.

Example:

apps/web → Next.js
apps/admin → React
apps/api → Go
workers/email → Python

Allow the user to select detected services.

Create optional configuration:

opencloud.yaml

Example:

services:
  web:
    root: apps/web
    type: web
    port: 3000

  api:
    root: apps/api
    type: web
    port: 8080

  worker:
    root: workers/email
    type: worker

opencloud.yaml is optional.

Auto-detection remains the default experience.

============================================================
10. ENVIRONMENTS
============================================================

Eventually support:

- production
- preview
- development

Environment variables and secrets are environment-scoped.

Conceptually:

Project
├── Production
├── Preview
└── Development

Do not make preview deployments mandatory for the first implementation.

============================================================
11. ENVIRONMENT VARIABLES AND SECRETS
============================================================

Create a secure environment/secrets system.

Normal variable:

NODE_ENV=production

Secret:

DATABASE_URL=*****
API_KEY=*****

Requirements:

- tenant-scoped
- service-scoped
- environment-scoped
- encrypted
- never logged
- never returned accidentally
- redact at log boundary
- explicit rotation
- access audit
- no NEXT_PUBLIC exposure unless intentionally configured by user

Preserve OpenCloud's existing secret-management/security philosophy.

OpenBao may be evaluated later for full secret custody and dynamic credentials,
but do not introduce it prematurely.

============================================================
12. LOGGING — VERCEL-LIKE USER EXPERIENCE
============================================================

User-facing logs are a CORE Phase 4 feature.

Not the same as internal operator audit logs.

Expose four conceptual log sources:

1. Build Logs
2. Runtime Logs
3. Request Logs
4. Platform Activity

Build Logs:

Cloning repository
Detected runtime
Installing dependencies
Building
Creating OCI image
Pushing image
Starting deployment
Health check

Runtime logs:

capture application stdout/stderr regardless of language.

Examples:

Node console.log
Go log/slog
PHP error_log
Python logging
Java stdout
Rust tracing/stdout

Request Logs:

generated from Caddy/ingress.

Include safe metadata such as:

timestamp
request_id
account_id
project_id
service_id
deployment_id
environment
method
path
status
duration
response_size

Do not expose sensitive headers, cookies, authorization tokens, or secrets.

Platform Activity:

deployment queued
deployment started
container created
health check passed
traffic switched
rollback
database created
bucket created
etc.

============================================================
13. LOGGING TECHNOLOGY
============================================================

Preferred architecture for MVP:

containers stdout/stderr
        ↓
Vector or Grafana Alloy
        ↓
Loki
        ↓
OpenCloud Go Logs API
        ↓
SSE
        ↓
Next.js Logs UI

Do not write a custom container log collector unless truly necessary.

Do not put millions of customer log lines in the OpenCloud control-plane
PostgreSQL database.

PostgreSQL stores:

- deployments
- events
- log stream metadata
- retention configuration
- audit logs

Log storage stores:

- application logs
- build logs
- request logs

Every runtime container must receive OpenCloud ownership metadata/labels.

Example:

opencloud.account_id
opencloud.project_id
opencloud.service_id
opencloud.deployment_id
opencloud.environment

Tenant filtering MUST happen server-side.

Never trust account_id passed from frontend.

============================================================
14. LIVE LOGS
============================================================

Implement user live-tail using Server-Sent Events first.

Example:

GET /api/v1/projects/{project_id}/logs/stream

Support UI controls:

- Live
- Pause
- Autoscroll
- Wrap lines
- timestamps
- filters

Log filters:

Environment
Service
Source
Level
HTTP status
Deployment
Time
Search

Possible tabs:

All
Build
Runtime
Requests
Platform

============================================================
15. REQUEST CORRELATION
============================================================

Propagate request IDs through:

Caddy
→ application request metadata where possible
→ OpenCloud API
→ logs

Allow the user to click a request log and see:

Request ID
Deployment
Service
Environment
Method
Path
Status
Duration
Timestamp
related safe application logs

Do not expose infrastructure secrets.

============================================================
16. OBSERVABILITY
============================================================

Customer observability belongs in Phase 4.

Operator/platform observability can continue in the later operations phase.

OpenTelemetry should be supported progressively.

Possible pipeline:

Application
→ OTLP / stdout
→ OpenTelemetry Collector
→ Loki
→ Prometheus
→ tracing backend

Do not require customer applications to instrument OpenTelemetry to obtain basic
runtime logs.

============================================================
17. DATABASE PLATFORM
============================================================

Initial database architecture is SHARED DATABASE hosting.

Example:

MariaDB shared node
├── tenant database A
├── tenant database B
└── tenant database C

PostgreSQL shared node
├── tenant database A
├── tenant database B
└── tenant database C

Every tenant gets:

- isolated database
- isolated database user
- independent password
- least privilege
- connection quota
- storage quota
- account ownership
- backup ownership

A tenant must never see another tenant's database.

Control-plane PostgreSQL must NEVER be exposed as a customer database.

Future tiers:

Shared Database
High Availability Shared Database
Dedicated Database

Dedicated database is NOT necessary for first implementation.

============================================================
18. DATABASE HOSTNAMES
============================================================

Support usable database hostnames/connection strings.

Example:

MariaDB:

db.opencloud.example:3306

mysql://user:password@db.opencloud.example:3306/database

PostgreSQL:

postgresql://user:password@db.opencloud.example:5432/database

Design routing/proxy architecture so a stable hostname can eventually survive
database-node relocation.

Do not simply expose unrestricted raw database hosts.

Production external database access requires:

- verified TLS
- firewall restrictions
- rate/connection control
- optional IP allowlists
- proper authentication

============================================================
19. DATABASE MANAGER
============================================================

User specifically DOES NOT want pgAdmin.

Do NOT use pgAdmin.

Do NOT make CloudBeaver the default.

Desired model:

db.<platform-domain>

Dashboard button:

Open Database Manager

For MariaDB/MySQL:

phpMyAdmin is acceptable.

Architecture:

db.<platform-domain>
    ↓
Caddy
    ↓
OpenCloud Database Console Gateway
    ↓
phpMyAdmin internal
    ↓
customer MariaDB

phpMyAdmin must NOT have:

- platform root password
- control-plane database access
- unrestricted server selection
- arbitrary internal network access

Disable arbitrary host selection.

Do not expose phpMyAdmin directly to the internet without OpenCloud gateway controls.

============================================================
20. POSTGRESQL DATABASE UI
============================================================

For PostgreSQL, build an OpenCloud-native database manager experience inspired by
Supabase Studio.

Do NOT use pgAdmin.

The PostgreSQL manager should eventually support:

- schema/table browser
- table rows
- columns
- indexes
- basic relationships
- SQL Console
- backups
- connection info
- database size
- connections
- activity/insights

It should feel like part of OpenCloud, not like a foreign admin panel.

Do not blindly clone Supabase UI assets or branding.

Use Supabase Studio only as product inspiration.

============================================================
21. DATABASE CONSOLE SESSION
============================================================

Database Manager access should use temporary OpenCloud-managed sessions.

Conceptual schema:

database_console_sessions

Fields similar to:

id
account_id
database_id
actor_id
engine
status
expires_at
created_at
revoked_at

Never store the customer database password in this table.

Flow:

user clicks Open Database Manager
→ validate account ownership
→ validate permissions
→ create short-lived console session
→ open db.<platform-domain>
→ expire/revoke session automatically

Later, create a temporary DB user for console access.

Example expiration:

15–30 minutes.

Temporary user must only have access to the selected customer database.

Never grant:

SUPER
FILE
GRANT OPTION
global PROCESS
CREATE USER
cross-tenant database access

============================================================
22. SQL CONSOLE
============================================================

Add a native SQL Console.

Location:

Database
→ SQL Console

The user should be able to execute SQL against THEIR customer database.

Do NOT execute queries using the platform database administrator credential.

Use scoped customer or temporary console credentials.

Initial safe defaults:

- read-only mode initially
- statement timeout around 30 seconds
- result maximum around 1,000 rows
- query-size limit
- connection limit
- multi-statement disabled initially
- cancellation support
- transaction timeout

SQL Console UI:

Run
Explain
Format SQL
Cancel

Eventually allow:

SELECT
INSERT
UPDATE
DELETE
CREATE/ALTER schema operations

For destructive statements:

DROP
TRUNCATE
dangerous ALTER
large DELETE

require explicit confirmation.

Do NOT rely on SQL string parsing as the primary security boundary.

Database permissions are the primary security boundary.

============================================================
23. SQL QUERY LOGGING
============================================================

Never write full customer SQL statements into normal OpenCloud logs by default.

Queries may contain:

- passwords
- API tokens
- email
- private user data

Safe audit record can contain:

database_id
actor_id
statement_type
query_hash
duration
affected_rows
status
timestamp

Full query history, if implemented:

- user-visible only
- encrypted
- retention-limited
- optional

============================================================
24. SQL IMPORT/EXPORT
============================================================

Small SQL query:
run interactively.

Large SQL dump:

upload
→ async background job
→ progress
→ cancel support
→ final result

Never hold a long HTTP request while importing a massive SQL dump.

============================================================
25. DATABASE INSIGHTS
============================================================

Add product-level database insights progressively:

- active connections
- database size
- storage growth
- slow queries
- query duration
- failed connections
- read/write activity
- connection quota

Do not expose raw sensitive SQL unnecessarily.

============================================================
26. DATABASE BACKUP
============================================================

Customer database backups are required.

Eventually support:

- automatic daily backup
- manual backup
- restore
- restore into a new database
- download/export
- retention policy

Prefer restoring into a new database before destructive overwrite.

Point-in-time recovery can come later.

============================================================
27. DATABASE CONNECTION MANAGEMENT
============================================================

Shared database must enforce fair resource use.

Plan for:

- connection limits per tenant
- query timeout
- storage quota
- connection pooling
- rate limits
- slow query protection
- plan-based limits

Later support separate credentials such as:

application user
read-only user
migration user
temporary console user

============================================================
28. DATABASE MIGRATIONS DURING APP DEPLOY
============================================================

Eventually allow optional deployment migration commands.

Examples:

php artisan migrate --force

npx prisma migrate deploy

alembic upgrade head

Migration sequence:

build complete
→ run migration in controlled task
→ health check
→ switch traffic

Migration failure should prevent the new deployment from becoming active.

Design carefully because database migrations can be irreversible.

============================================================
29. PHP HOSTING
============================================================

PHP is a first-class workload.

Support progressively:

- PHP application
- Composer
- Laravel
- Symfony
- WordPress later

PHP deployment:

source
→ detection
→ composer install
→ build image
→ runtime
→ Caddy

Never expose PHP-FPM directly to the internet.

Caddy/web server should front it.

Separate immutable application source from mutable storage.

Example:

/app → deployment
/data/uploads → persistent
/data/storage → persistent

============================================================
30. STATIC HTML HOSTING
============================================================

Support HTML/CSS/JavaScript static sites.

Example:

index.html
css/
js/
images/

Static projects should have a lightweight deployment path.

They still receive:

- deployment revision
- domain
- HTTPS
- request logs
- rollback

============================================================
31. PERSISTENT STORAGE FOR APPLICATIONS
============================================================

Support persistent paths for applications that need mutable files.

Examples:

Laravel storage
WordPress uploads
user uploads

Do not make the entire deployment filesystem mutable.

Keep:

application image/source → immutable

persistent volume → mutable

Add quotas and backup policies.

============================================================
32. OBJECT STORAGE — SUPABASE-LIKE EXPERIENCE
============================================================

Create OpenCloud Object Storage.

Product experience inspired by Supabase Storage.

User model:

Project
└── Storage
    ├── avatars
    ├── documents
    └── product-images

Support:

- buckets
- public bucket
- private bucket
- upload
- download
- rename
- move
- delete
- virtual folders
- signed URL
- presigned upload
- storage quota
- MIME restrictions
- file size limit
- access keys
- activity
- usage
- bandwidth later

Do NOT store object binary data inside the control-plane PostgreSQL database.

============================================================
33. OBJECT STORAGE ARCHITECTURE
============================================================

Implement a provider abstraction.

For example:

type ObjectStorageProvider interface {
    CreateBucket(...)
    DeleteBucket(...)
    PutObject(...)
    GetObject(...)
    DeleteObject(...)
    PresignUpload(...)
    PresignDownload(...)
}

Implement:

- FakeStorageProvider
- S3StorageProvider

Use an S3-compatible backend.

Self-hosted backend can evaluate RustFS.

The architecture must NOT be locked to RustFS.

It should also work eventually with:

- AWS S3
- Cloudflare R2
- other S3-compatible providers

============================================================
34. OBJECT STORAGE DOMAINS
============================================================

Suggested public platform endpoints:

storage.<platform-domain>
- OpenCloud Storage REST/API

s3.<platform-domain>
- S3-compatible endpoint

cdn.<platform-domain>
- public object delivery/CDN later

Do not hardcode the real production domain.
Use configuration.

============================================================
35. DIRECT OBJECT UPLOAD
============================================================

Large file data should ideally NOT pass through the central OpenCloud API.

Flow:

browser
→ request upload authorization
→ OpenCloud validates tenant/quota/MIME/size
→ generate short-lived presigned upload URL
→ browser uploads directly to S3 backend
→ OpenCloud verifies completion

Support multipart/resumable upload later.

============================================================
36. OBJECT STORAGE ACCESS KEYS
============================================================

Never give customers a global platform S3 credential.

Create scoped credentials:

account/project/service/bucket permissions.

Example:

frontend → public read
api → read/write selected bucket
worker → read/write documents

Secret key should be:

- shown once
- encrypted
- revocable
- rotatable
- audited

============================================================
37. STORAGE SECURITY
============================================================

Validate:

- content length
- MIME type
- actual content signature where appropriate
- filename/key policy
- path traversal
- quota
- tenant ownership

Plan malware scanning for customer uploads before fully public availability.

Later:

- object versioning
- lifecycle policies
- image transformation
- replication
- CDN
- webhook/event after upload

============================================================
38. FRONTEND DESIGN DIRECTION
============================================================

IMPORTANT VISUAL REQUIREMENT:

Use the supplied OpenCloud UI mockup image as the PRIMARY visual reference.

The UI should be modern, dark, elegant, infrastructure-oriented, and polished.

It is inspired by Railway/Vercel but must NOT be a pixel-for-pixel clone.

Keep OpenCloud branding.

Visual language:

- very dark navy/black backgrounds
- slightly lighter cards
- subtle borders
- purple/violet accent
- green active states
- blue/cyan secondary technical accents where appropriate
- generous spacing
- soft rounded cards
- minimal visual noise
- high information density without looking like cPanel
- desktop-first but responsive on mobile
- professional cloud-platform feeling

Do not copy Railway logos, proprietary assets, wording, or exact component design.

============================================================
39. MAIN SIDEBAR
============================================================

Design a persistent application sidebar similar to the approved reference.

Suggested items:

OpenCloud logo

Workspace selector

Projects
Databases
Storage
Domains
Members
Billing
Templates

bottom:

Docs
Support
Settings

user account

Only render sections that actually exist or mark unfinished product areas clearly.

============================================================
40. CREATE NEW PROJECT PAGE
============================================================

Create a polished page:

Create New Project

Primary input:

"Describe your project or paste a repository URL"

This may later support AI/project intent detection, but do NOT make AI a blocker.

Cards:

GitHub Repository
Database
Template
Docker Image
Function
Bucket
Static Upload if appropriate
Empty Project

IMPORTANT:
Some options may be marked Coming Soon if backend support is intentionally deferred.

Function/serverless is NOT an immediate backend priority.

Do not fake functionality.

============================================================
41. PROJECT ARCHITECTURE CANVAS
============================================================

This is one of the key OpenCloud UX features.

Project page default tab:

Architecture

Other tabs:

Deployments
Logs
Metrics
Domains
Environment
Settings

Architecture tab displays service/resource nodes visually.

Use React Flow or another mature node graph library.

Do NOT build graph drag/zoom/edge behavior from scratch.

Example nodes:

frontend
Next.js · Web
Active

backend
Go · API
Active

postgres
PostgreSQL · Database
Active

redis
Redis · Cache
Active

bucket
Object Storage
Active

Edges represent application/resource relationships.

Support:

- pan
- zoom
- fit view
- optional minimap
- draggable layout
- node click
- status
- resource type
- deployment revision
- quick action
- activity panel

Node click should open a drawer or detail panel.

============================================================
42. PROJECT OVERVIEW
============================================================

Create a dashboard-style overview.

Example information:

Services
Deployments
Requests
Bandwidth

Recent deployments

Resource usage:

CPU
Memory
Storage
Bandwidth

Keep values real.
Do not display fake metrics when unavailable.

============================================================
43. DATABASE DETAIL UI
============================================================

Database page should visually match the approved mockup.

Tabs may include:

Overview
Data / SQL
Backups
Settings

Show:

engine
status
connections
database size
storage quota
backup state

Primary action:

Open Database Manager

For PostgreSQL, native OpenCloud Studio-like UI.

For MariaDB, phpMyAdmin via secure gateway.

============================================================
44. STORAGE UI
============================================================

Storage page should look like a modern file/object browser.

Tabs:

Files
Settings
Access Keys

Support:

Upload
New Folder
Delete
Rename
Move
Copy URL
Generate signed URL

File table:

Name
Size
Modified
Visibility where appropriate

Show bucket storage usage.

============================================================
45. LOGS UI
============================================================

Logs UI should resemble the approved mockup and modern Vercel-style log experience.

Header:

service name
runtime
status

Tabs:

Logs
Metrics
Settings

Controls:

source filter
level filter
deployment filter
time filter
search
Live toggle
Pause
clear local view
full screen

Rows:

timestamp
level
message
request method/path where relevant
duration/status where relevant

Use monospace for log body.

Optimize rendering for thousands of rows using virtualization.

============================================================
46. ACTIVITY UI
============================================================

Architecture screen can include a small Activity panel.

Examples:

Deployed frontend
Deployed backend
Database backup
Scaled backend
Domain attached
Deployment rolled back

Do not put secrets or raw provider errors here.

============================================================
47. FRONTEND STACK
============================================================

Preserve current Next.js/TypeScript architecture.

Use existing design system when practical.

Preferred additions where compatible:

- Tailwind CSS
- shadcn/ui or existing component primitives
- React Flow for architecture graph
- a mature code editor such as Monaco for SQL Console only if dependency cost is justified
- virtualization for logs/file tables if necessary

Do not introduce large dependencies without first checking CLAUDE.md dependency rules.

Before adding any dependency:
explain why it is necessary and verify whether repository approval is required.

============================================================
48. REAL DATA, NOT MOCK PRODUCT
============================================================

The UI must connect to real APIs progressively.

Do not create a beautiful dashboard that is backed permanently by fake JSON.

Mock/fake providers are acceptable for tests and incremental development.

Production UI should show:

loading
empty
error
permission
not supported yet

states clearly.

============================================================
49. BACKEND ARCHITECTURE RULE
============================================================

Continue the existing layering:

handler
→ service
→ repository
→ database

and:

service / worker
→ provisioner / build / registry / storage providers

Do NOT:

- put SQL in handlers
- put Gin contexts into repositories
- let frontend call infrastructure directly
- put Docker behavior in the dashboard
- let services bypass account scoping
- hold SQL transactions open during long external provider calls

Long-running work goes through the async job system.

============================================================
50. NEW BACKEND MODULES
============================================================

Introduce modules incrementally such as:

internal/build/
internal/registry/
internal/deployment/
internal/logs/
internal/storage/
internal/databaseconsole/
internal/telemetry/

Potential future modules:

internal/scanner/
internal/signing/
internal/policy/
internal/secrets/

Do not create empty abstractions with no real use.

============================================================
51. SECURITY SCANNING
============================================================

After core build/deploy works, integrate progressively:

Trivy
- vulnerability scanning
- SBOM
- misconfiguration scanning where relevant

Cosign
- OCI image signing

Possible pipeline:

build
→ push
→ scan
→ policy
→ sign
→ deploy

Do not make these block initial development before the basic deployment path is proven,
but design the state machine to accommodate them.

Pin versions/digests.
Do not use floating latest tags for security-sensitive infrastructure.

============================================================
52. POLICY ENGINE
============================================================

OPA may be evaluated later as deployment policy complexity grows.

Potential policies:

- privileged container prohibited
- host mount prohibited
- required CPU/memory limits
- image digest required
- image signature required
- vulnerability threshold
- allowed ports
- plan limits
- preview TTL

Do not add OPA prematurely if simple typed Go policy checks are sufficient.

============================================================
53. WORKFLOW ENGINE
============================================================

Continue using OpenCloud's PostgreSQL-backed job queue initially.

Do NOT replace it with Temporal now.

Temporal may be evaluated later if deployment workflow becomes sufficiently complex:

clone
detect
build
push
scan
sign
deploy
health check
traffic switch
monitor
rollback
cleanup

Only introduce Temporal after demonstrated need.

============================================================
54. AUTHORIZATION EVOLUTION
============================================================

Current customer/admin model remains valid for MVP.

OpenFGA or equivalent may be evaluated later for:

organizations
teams
project roles
viewer/developer/billing roles

Do not introduce that complexity now.

============================================================
55. HESTIA
============================================================

Do not make Hestia the core platform architecture.

If current Hestia fallback remains:

- keep it isolated
- keep it provider-based
- use it only where appropriate for classic hosting compatibility

Universal application deployment should use the OpenCloud Docker/OCI platform path.

============================================================
56. ROADMAP REORDERING
============================================================

The next major roadmap should prioritize:

PHASE 4 — UNIVERSAL APPLICATION PLATFORM

Phase 4A:
Projects, services, deployments

Phase 4B:
Git/ZIP source acquisition

Phase 4C:
Railpack build detection

Phase 4D:
Isolated BuildKit builder

Phase 4E:
Private OCI registry

Phase 4F:
Immutable deployment revisions + health checks + rollback

Phase 4G:
Build/runtime/request/platform logs

Phase 4H:
Environment variables + secrets

Phase 4I:
PHP + persistent storage

Phase 4J:
Database Manager + SQL Console

Phase 4K:
Monorepo support

Phase 4L:
Object Storage

Phase 4M:
Preview deployments

Do not implement all items simultaneously.

============================================================
57. LATER PHASES
============================================================

PHASE 5:
Usage metering
Quotas
Plans
Billing
Subscriptions

Meter reliably before charging.

Potential usage:

CPU
RAM
storage
bandwidth
build minutes
database size
database connections
object storage
log ingestion

PHASE 6:
Production operations

Prometheus
Grafana
alerts
central operator logs
backup
off-host backup
restore rehearsals
node draining
incident runbooks
security hardening
capacity management

PHASE 7:
Scale

advanced scheduler
autoscaling
scale-to-zero
serverless functions
multi-region
distributed build cache
organizations/team permissions
advanced workflows

============================================================
58. FEATURES TO DEFER
============================================================

Do NOT make these immediate priorities:

- full email hosting
- self-operated SMTP platform
- reseller features
- Kubernetes rewrite
- serverless functions
- multi-region
- complex marketplace
- advanced organization RBAC
- full Temporal migration
- autoscaling
- scale-to-zero
- huge billing system

Focus first on the core deploy experience.

============================================================
59. FIRST TEST MATRIX
============================================================

The universal deployment path should eventually prove:

1. Static HTML/CSS/JS
2. React/Vite
3. Next.js
4. Go API
5. PHP basic app
6. Laravel
7. Python/FastAPI
8. React + Go monorepo
9. custom Dockerfile after isolated builder is secure

Tests should prove the generic architecture.

Do not implement separate deployment pipelines for every language.

============================================================
60. TESTING REQUIREMENTS
============================================================

For every backend feature:

- unit tests
- tenant-isolation tests
- validation tests
- permission tests
- failure/retry tests
- idempotency tests where appropriate

For provider integrations:
provide fake providers for CI.

CI must not depend on a production Docker daemon, database node, or object storage.

For migrations:
test up
test constraints
test relevant down path for disposable development if repository policy allows
verify existing data preservation

For frontend:
test critical state transitions
empty state
loading state
error state
permission state
responsive behavior

============================================================
61. FAILURE DESIGN
============================================================

Every distributed operation must handle:

- retry
- timeout
- cancellation
- worker crash
- API restart
- duplicate job delivery
- partial external success
- stale state

Provider operations should be idempotent wherever possible.

Never mark an external operation successful in UI before durable state is recorded.

============================================================
62. ERROR UX
============================================================

Never show customers:

- raw SQL errors containing internal identifiers
- stack traces
- Docker errors
- internal IPs
- secret values
- provider credentials
- Caddy internal addresses

Return stable customer-facing errors.

Log detailed internal errors only after redaction.

============================================================
63. UI REFERENCE PRIORITY
============================================================

The supplied OpenCloud mockup is the visual target.

It contains approximately:

LEFT:
- OpenCloud sidebar
- Create New Project experience

RIGHT:
- Project Architecture visual canvas
- frontend/backend/database/cache/storage nodes
- activity panel

BOTTOM:
- Project Overview card
- Database Detail card
- Storage Bucket browser
- Logs viewer

Reproduce the design language, hierarchy, spacing, dark palette, purple accents,
card style, and infrastructure clarity.

Do NOT copy Railway trademarks or exact proprietary assets.

The final result should look like a coherent OpenCloud product.

============================================================
64. IMPLEMENTATION STRATEGY
============================================================

DO NOT attempt a giant rewrite.

Work in PR-sized vertical slices.

Recommended first implementation sequence:

SLICE 1 — PROJECT DOMAIN MODEL ✅ COMPLETE

Add:

projects
services
deployments
deployment_events
source_artifacts where required

Implement:

migration
models
repositories
services
handlers/API
tenant isolation tests
frontend basic project screens

Preserve sites compatibility.

STOP and report.

SLICE 2 — BUILD ABSTRACTION ✅ COMPLETE

Add:

BuildProvider
FakeBuildProvider
StaticBuildProvider
Railpack plan/detection integration

Do not enable unsafe builds yet.

Tests.

STOP and report.

SLICE 3 — ISOLATED BUILDER ✅ COMPLETE (SAFE FOUNDATION)

Add:

dedicated builder command/service
BuildKit integration
resource/time limits
build state machine
build log streaming
cleanup

Tests.

STOP and report.

SLICE 4 — REGISTRY + DEPLOYMENTS ✅ COMPLETE (SAFE FOUNDATION)

Add:

RegistryProvider
OCI digest storage
immutable revisions
runtime deployment
health checking
Caddy traffic switch
rollback

Tests.

STOP and report.

SLICE 5 — LOGS ✅ COMPLETE (SAFE FOUNDATION)

Add:

log metadata contract
collector integration
Loki
Go Logs API
SSE
Next.js Logs Viewer

Tests.

STOP and report.

SLICE 6 — ENV/SECRETS ✅ COMPLETE

Add safe environment variables and secret delivery.

STOP and report.

SLICE 7 — DATABASE MANAGER ✅ COMPLETE (SAFE FOUNDATION)

Add:

db.<platform-domain>
database console session
phpMyAdmin for MariaDB
native PostgreSQL management UI foundation
SQL Console read-only first

STOP and report.

SLICE 8 — OBJECT STORAGE ✅ COMPLETE (SAFE FOUNDATION)

Add:

bucket model
S3 provider
signed URLs
upload UI
storage browser
quota

STOP and report.

Continue only after each slice is stable.

============================================================
65. AGENT WORKFLOW
============================================================

For EACH slice:

1. Inspect the existing relevant code.
2. Explain what currently exists.
3. Identify architectural conflicts.
4. Propose exact change.
5. List migrations.
6. List API changes.
7. List backend packages/files.
8. List frontend pages/components.
9. List security implications.
10. Implement the smallest complete vertical slice.
11. Run formatting.
12. Run tests.
13. Run lint/static analysis already defined by repository.
14. Report failures honestly.
15. Summarize changed files.
16. Explain manual testing steps.
17. List unresolved risks.
18. STOP BEFORE COMMIT OR PUSH.

Never claim tests passed unless they were actually run.

Never silently weaken a security control to make a test pass.

============================================================
66. CODE QUALITY
============================================================

Follow existing Go and TypeScript conventions.

Prefer:

- explicit types
- small interfaces
- dependency injection
- context.Context for I/O
- structured logging
- typed application errors
- idempotent external operations
- bounded pagination
- transaction safety
- clear API contracts

Avoid:

- giant services
- generic god classes
- global mutable state
- hidden cross-tenant queries
- unbounded list endpoints
- raw string-built SQL
- shell interpolation
- secrets in code
- security TODOs that silently ship

============================================================
67. FINAL PRODUCT EXPERIENCE
============================================================

The final OpenCloud product should eventually allow this experience:

User logs into OpenCloud.

Clicks:

Create New Project

Chooses:

GitHub Repository

OpenCloud discovers:

frontend → Next.js
backend → Go
database → PostgreSQL
bucket → Object Storage

Architecture Canvas shows all resources.

The user watches Build Logs live.

Deployment becomes Ready.

OpenCloud provides:

HTTPS domain
custom domain
runtime logs
request logs
metrics
environment variables
database
database manager
SQL Console
object storage
backup
rollback

A PHP customer should also be able to:

upload/connect PHP app
use MariaDB
open phpMyAdmin
upload persistent files
use SQL
see logs
use custom domain + HTTPS

The product should feel modern like a cloud platform, not like an old cPanel clone.

============================================================
68. IMMEDIATE TASK
============================================================

Do NOT start by implementing every feature in this document.

Start with:

PHASE 4A / SLICE 1:
PROJECTS + SERVICES + DEPLOYMENTS + DEPLOYMENT EVENTS.

Before writing code:

inspect the repository,
especially CLAUDE.md, current Site model, API conventions, migration conventions,
tenant isolation, frontend dashboard, and provisioner boundaries.

Then provide:

A. current-state assessment
B. proposed schema
C. compatibility strategy with sites
D. API design
E. frontend route/component plan
F. test plan
G. security impact
H. exact files expected to change

After presenting that plan, implement the approved slice in the working tree.

DO NOT COMMIT.
DO NOT PUSH.
