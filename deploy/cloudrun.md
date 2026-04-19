# Google Cloud Run deployment

Cloud Run is a great fit for this app because the API is stateless. TimescaleDB
is **not** a managed offering on GCP — pick one of:

- [Timescale Cloud](https://www.timescale.com/cloud) (recommended; works anywhere)
- Cloud SQL for Postgres + `CREATE EXTENSION timescaledb` (community/Apache-2 build only — feature-limited; check current support)
- Self-hosted TimescaleDB on a Compute Engine VM
- Crunchy Bridge / Aiven Postgres with the Timescale extension enabled

## Build & push the image

```bash
PROJECT_ID=your-gcp-project
REGION=us-central1

gcloud auth configure-docker $REGION-docker.pkg.dev
docker build -t $REGION-docker.pkg.dev/$PROJECT_ID/agent/api:latest -f backend/Dockerfile backend
docker push $REGION-docker.pkg.dev/$PROJECT_ID/agent/api:latest
```

## Run migrations as a Cloud Run Job

```bash
gcloud run jobs create agent-migrate \
  --image $REGION-docker.pkg.dev/$PROJECT_ID/agent/api:latest \
  --region $REGION \
  --command /bin/migrate --args up \
  --set-secrets DATABASE_URL=DATABASE_URL:latest

gcloud run jobs execute agent-migrate --region $REGION --wait
```

## Deploy the API

```bash
gcloud run deploy agent-api \
  --image $REGION-docker.pkg.dev/$PROJECT_ID/agent/api:latest \
  --region $REGION \
  --port 8080 \
  --cpu 1 --memory 512Mi \
  --min-instances 0 --max-instances 10 \
  --allow-unauthenticated \
  --set-secrets ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest,DATABASE_URL=DATABASE_URL:latest,JWT_SECRET=JWT_SECRET:latest \
  --set-env-vars APP_URL=https://agent-api-xxxx-uc.a.run.app,ANTHROPIC_MODEL=claude-sonnet-4-5-20250929
```

## Notes for SSE on Cloud Run

The agent run endpoint streams Server-Sent Events. Cloud Run supports this
natively but the request timeout matters — bump `--timeout 600` (or higher) if
you expect long agent runs.
