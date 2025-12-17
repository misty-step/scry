#!/usr/bin/env npx tsx
/**
 * Create or update Scry prompts in Langfuse.
 *
 * Usage:
 *   npx tsx scripts/create-prompt.ts --all                    # Create all prompts
 *   npx tsx scripts/create-prompt.ts --name scry-intent-extraction
 *   npx tsx scripts/create-prompt.ts --name scry-intent-extraction --label production
 */

import * as fs from "fs/promises";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const FETCH_TIMEOUT_MS = 30_000;

const PROMPTS = [
  { name: "scry-intent-extraction", file: "intent-extraction.txt" },
  { name: "scry-concept-synthesis", file: "concept-synthesis.txt" },
  { name: "scry-phrasing-generation", file: "phrasing-generation.txt" },
];

function getCredentials(): { secretKey: string; publicKey: string; baseUrl: string } {
  const secretKey = process.env.LANGFUSE_SECRET_KEY;
  const publicKey = process.env.LANGFUSE_PUBLIC_KEY;
  const baseUrl = (process.env.LANGFUSE_HOST || "https://cloud.langfuse.com").replace(/\/+$/, "");

  if (!secretKey || !publicKey) {
    console.error("Error: LANGFUSE_SECRET_KEY and LANGFUSE_PUBLIC_KEY must be set");
    process.exit(1);
  }

  return { secretKey, publicKey, baseUrl };
}

function parseArgs(): { all: boolean; name?: string; label?: string } {
  const args = process.argv.slice(2);
  const result: { all: boolean; name?: string; label?: string } = { all: false };

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--all") {
      result.all = true;
    } else if (args[i] === "--name" && args[i + 1]) {
      result.name = args[i + 1];
      i++;
    } else if (args[i] === "--label" && args[i + 1]) {
      result.label = args[i + 1];
      i++;
    }
  }

  return result;
}

async function createPrompt(
  name: string,
  promptContent: string,
  labels: string[]
): Promise<void> {
  const { secretKey, publicKey, baseUrl } = getCredentials();
  const authHeader = Buffer.from(`${publicKey}:${secretKey}`).toString("base64");
  const apiUrl = `${baseUrl}/api/public/v2/prompts`;

  const body = {
    type: "text",
    name,
    prompt: promptContent,
    labels,
    config: {},
  };

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

  const response = await fetch(apiUrl, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Basic ${authHeader}`,
    },
    body: JSON.stringify(body),
    signal: controller.signal,
  }).finally(() => clearTimeout(timeoutId));

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`API error ${response.status}: ${errorText}`);
  }

  const result = await response.json();
  console.log(`  Created: ${name} (version ${result.version})`);
}

async function main() {
  const { all, name, label } = parseArgs();
  const labels = label ? [label] : [];
  const promptsDir = path.join(__dirname, "../prompts");

  if (!all && !name) {
    console.error("Error: --all or --name is required");
    console.log("Usage:");
    console.log("  npx tsx scripts/create-prompt.ts --all");
    console.log("  npx tsx scripts/create-prompt.ts --name scry-intent-extraction");
    process.exit(1);
  }

  const toCreate = all
    ? PROMPTS
    : PROMPTS.filter((p) => p.name === name);

  if (toCreate.length === 0) {
    console.error(`Error: Unknown prompt name: ${name}`);
    console.log("Available prompts:", PROMPTS.map((p) => p.name).join(", "));
    process.exit(1);
  }

  console.log(`Creating ${toCreate.length} prompt(s) in Langfuse...`);

  for (const prompt of toCreate) {
    const filePath = path.join(promptsDir, prompt.file);
    const content = await fs.readFile(filePath, "utf-8");
    await createPrompt(prompt.name, content, labels);
  }

  console.log("Done.");
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
