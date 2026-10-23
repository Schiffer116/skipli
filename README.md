# Real-Time Board Management Tool

![Watch the demo](demo.gif)

## Overview

This project is a **real-time board management tool** that allows teams to collaborate on tasks and cards. Features include:

- Multi-user support with authentication
- Board, Card and Task creation, updates, deletion
- Drag-and-drop task management
- Real-time updates via WebSockets
- Invite members to boards with email notifications

## Tech Stack

- **Frontend**: React, TypeScript, TailwindCSS (`ui/`)
- **Backend**: Go, DynamoDB (`app/`)
- **Infrastructure**: Terraform on AWS: S3 + CloudFront, API Gateway, Lambda, DynamoDB, Cognito (`infra/`)

## Architecture

![AWS architecture](infra/architecture.png)

Rendered by [TerraVision](https://github.com/patrickchugh/terravision) from a
plan of the dev environment (needs AWS access, Graphviz and `uv`). Labels and
arrows are curated in `infra/terravision.yml`, and `infra/diagram.gvpr` lays
it out left to right, one column per tier. To regenerate:

```bash
cd infra && make diagram
```

## CLI

`skipli-cli` reads and changes boards from a terminal, mostly so agents can
work on them. Install it with `make -C api install-cli`, sign in with
`skipli-cli login <email>` (Production; `--env dev` for Development), and run
`skipli-cli help` for the commands. Boards, cards and tasks are named by id
(any unique prefix works); `--json` prints JSON.

A board can also live in a file (`skipli-board/1`, JSON, stable for git). A
board argument with a `/` or ending in `.json` is a file, and every board
command works on it offline, with no sign-in: `skipli-cli new-board --file
army.skipli.json Army`, then `skipli-cli add-card army.skipli.json Backlog`.
`export <board> [file]` and `import <file>` move a board between a file and
your account; `boards <dir>` lists the board files in a directory.

## Running locally

1. Clone the repository:

```bash
git clone https://github.com/Schiffer116/skipli.git
cd skipli
```

1. Configure the API's environment (AWS credentials come from the usual
   AWS CLI config/profile; the table is in `ap-southeast-7`):

```bash
export APP_EMAIL=your-app-email@example.com
export APP_EMAIL_PASSWORD=your-app-email-password
export JWT_SECRET=a-string-secret-at-least-256-bits-long
export TABLE_NAME=Skipli
```

1. Start the API (port 3000, override with `PORT`):

```bash
cd api && make run
```

1. Start the frontend (proxies `/api` to the API, override with `API_URL`):

```bash
cd ui && pnpm install && pnpm dev
```

## Deploying

With the same environment variables set and AWS credentials for the target
account:

```bash
./infra/deploy.sh
```

This deploys `infra/template.yaml`: the frontend on S3 behind CloudFront and
the API on Lambda under `/api/*` on the same domain. It reuses an existing
`Skipli` table by default; set `CREATE_TABLE=true` to have the stack create
it. `STACK_NAME` and `AWS_REGION` are also configurable.

The site is served at `https://skipli.schifferarchitecture.com`: the script
issues its certificate in `us-east-1` (`infra/certificate.yaml`, as CloudFront
requires) and creates the DNS records in the domain's Route 53 zone. Set
`DOMAIN_NAME` to use another domain, or to an empty string for none.

## CI/CD

`.github/workflows/ci.yaml` runs on every push and pull request:
frontend lint + unit tests + typecheck + build, backend `gofmt`/`vet`/`test`/Lambda build, and
lint for the CloudFormation templates and deploy script. Pushes to `master`
that pass then deploy with `infra/deploy.sh`.

The deploy job authenticates to AWS with GitHub OIDC, so no AWS keys are
stored in GitHub. One-time setup:

1. Deploy the bootstrap stack (set `CreateOidcProvider=false` if the account
   already has a `token.actions.githubusercontent.com` identity provider):

```bash
aws cloudformation deploy --region ap-southeast-7 \
  --stack-name skipli-pipeline \
  --template-file infra/pipeline.yaml \
  --capabilities CAPABILITY_NAMED_IAM
aws cloudformation describe-stacks --region ap-southeast-7 \
  --stack-name skipli-pipeline --query 'Stacks[0].Outputs'
```

1. In the GitHub repository, create an environment named `production` and
   add to it:
   - variables `AWS_DEPLOY_ROLE_ARN` and `AWS_CFN_ROLE_ARN` (the two outputs)
   - secrets `APP_EMAIL`, `APP_EMAIL_PASSWORD` and `JWT_SECRET`

## Screenshots

![workspace](workspace.png)
![board](board.png)
