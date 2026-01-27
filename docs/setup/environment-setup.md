# Environment Setup Guide

This guide helps you configure environment variables for the Scry application across different environments.

**Last Updated**: January 2026
**Tech Stack**: Next.js 15, Convex (backend), Clerk (auth), Google Gemini AI

## Quick Start

1. **Copy the template**:
   ```bash
   cp .env.example .env.local
   ```

2. **Fill in your values** in `.env.local`

3. **Validate configuration**:
   ```bash
   pnpm env:validate
   ```

## Required Environment Variables

### Core Application

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `GOOGLE_AI_API_KEY` | Google AI API key for quiz generation | `AIzaSy...` | ✅ |
| `NEXT_PUBLIC_CONVEX_URL` | Convex deployment URL | `https://excited-penguin-123.convex.cloud` | ✅ |
| `CONVEX_DEPLOY_KEY` | Convex deployment key (for builds) | `prod:abc123...` | ✅ (Vercel only) |

### Clerk Authentication

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | Clerk publishable key | `pk_test_...` | ✅ |
| `CLERK_SECRET_KEY` | Clerk secret key | `sk_test_...` | ✅ |
| `CLERK_WEBHOOK_SECRET` | Webhook signing secret | `whsec_...` | ✅ (prod) |

## Setting Environment Variables

### Local Development (.env.local)

```bash
# Core required variables
GOOGLE_AI_API_KEY=your-google-ai-api-key
NEXT_PUBLIC_CONVEX_URL=https://your-deployment.convex.cloud

# Optional: Only needed if running builds locally
CONVEX_DEPLOY_KEY=prod:your-deploy-key
```

### Convex Environment Variables

Set these in Convex dashboard (not in `.env.local`):

```bash
# Set production environment variables
npx convex env set GOOGLE_AI_API_KEY "your-google-ai-key" --prod
npx convex env set AI_MODEL "google/gemini-3-flash-preview" --prod

# Verify they're set
npx convex env list --prod
```

### Vercel Environment Variables

Add these in Vercel dashboard → Settings → Environment Variables:

1. **Production Environment**:
   - `GOOGLE_AI_API_KEY`
   - `NEXT_PUBLIC_CONVEX_URL`
   - `CONVEX_DEPLOY_KEY`

2. **Preview Environment** (Free Convex Tier):
   - Same as production (our build script handles preview deployments safely)

## Environment-Specific Setup

### Development

1. **Start Convex dev server** (required for local development):
   ```bash
   npx convex dev
   ```
   This will:
   - Connect to your Convex deployment
   - Generate TypeScript types
   - Watch for function changes
   - Show the deployment URL

2. **Start Next.js dev server** (in another terminal):
   ```bash
   pnpm dev
   ```

### Production

1. **Deploy Convex functions**:
   ```bash
   npx convex deploy --prod
   ```

2. **Deploy to Vercel**:
   ```bash
   vercel --prod
   ```

## Getting Environment Values

### Google AI API Key

1. Go to https://makersuite.google.com/app/apikey
2. Click "Create API Key"
3. Copy the key (starts with `AIzaSy`)
4. Optional: Add API restrictions for security

### Convex Configuration

1. **Create Convex account**: https://convex.dev
2. **Run setup**:
   ```bash
   npx convex dev
   ```
3. **Get deployment URL**: Shown in terminal or Convex dashboard
4. **Generate deploy key**: Dashboard → Settings → Deploy Keys

### Clerk Configuration

1. **Create account**: https://clerk.com
2. **Create application**: Dashboard → Applications → Create application
3. **Get API keys**: Dashboard → API Keys
4. **Copy keys**: `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` and `CLERK_SECRET_KEY`
5. **Configure webhook**: Dashboard → Webhooks (set `CLERK_WEBHOOK_SECRET`)

## Troubleshooting

### "NEXT_PUBLIC_CONVEX_URL is not defined"
- Ensure you've added it to `.env.local`
- Check the URL format: `https://[deployment].convex.cloud`
- Restart your dev server after adding

### "Authentication not working"
- Verify Clerk keys are set correctly in `.env.local`
- Check that `CLERK_WEBHOOK_SECRET` matches Clerk dashboard
- Ensure Clerk application is configured for your domain

### "Convex functions not updating"
- Make sure `npx convex dev` is running
- Check for TypeScript errors in convex/ directory
- Try `npx convex deploy --prod` for production

## Security Best Practices

1. **Never commit `.env.local`** - It's in .gitignore by default
2. **Use different API keys** for development and production
3. **Rotate keys regularly** - Especially if exposed
4. **Limit API key permissions** - Use minimum required access
5. **Monitor usage** - Check dashboards for unusual activity

### Development

```bash
# Local development with .env.local
cp .env.example .env.local
# Edit .env.local with your development values
pnpm env:validate
```

### Preview/Staging

```bash
# Validate preview environment
pnpm env:validate:preview
```

### Production

```bash
# Validate production readiness
pnpm env:validate:prod
pnpm deploy:check
```

## Validation Commands

| Command | Purpose |
|---------|---------|
| `pnpm env:validate` | Validate current environment |
| `pnpm env:validate:prod` | Validate for production |
| `pnpm env:validate:preview` | Validate for preview |
| `pnpm env:example` | Regenerate .env.example |
| `pnpm deploy:check` | Full deployment readiness check |

## Security Best Practices

### ✅ DO

- Use `.env.local` for local development
- Generate strong secrets: `openssl rand -base64 32`
- Use HTTPS URLs in production
- Keep API keys private and rotate regularly
- Validate environment before deployment

### ❌ DON'T

- Commit `.env` files with real secrets
- Use placeholder values in production
- Share API keys in chat/email
- Use the same secrets across environments
- Skip environment validation

## Getting API Keys

### Google AI API Key

1. Visit [Google AI Studio](https://aistudio.google.com/app/apikey)
2. Create a new API key
3. Copy the key starting with `AIzaSy...`

### Clerk API Keys

1. Visit [Clerk Dashboard](https://dashboard.clerk.com)
2. Select your application
3. Go to API Keys section
4. Copy `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` and `CLERK_SECRET_KEY`

### Vercel KV (Optional)

1. Visit Vercel Dashboard → Storage → KV
2. Create a new KV database
3. Copy all three connection variables
4. Use for rate limiting features

## Troubleshooting

### Common Issues

**Environment variables not loading**:
- Ensure `.env.local` exists and has correct format
- Check for syntax errors (no spaces around `=`)
- Restart development server after changes

**API key validation fails**:
- Verify key format matches requirements
- Check for trailing whitespace or quotes
- Regenerate key if format seems incorrect

**Database connection fails**:
- Verify connection string format
- Check network connectivity
- Ensure database is running and accessible

**Authentication fails**:
- Verify Clerk API keys are valid
- Check that webhook secret matches
- Ensure Clerk application is properly configured

### Getting Help

1. Run validation: `pnpm env:validate`
2. Check deployment readiness: `pnpm deploy:check`
3. Review error messages and fix issues
4. Consult documentation for specific services

## Environment File Hierarchy

```
.env.example          # Template (committed to git)
.env.local           # Local development (gitignored)
.env                 # Shared defaults (should be gitignored)
.env.production      # Production overrides (on Vercel)
.env.preview         # Preview overrides (on Vercel)
```

Only `.env.example` should be committed to version control.