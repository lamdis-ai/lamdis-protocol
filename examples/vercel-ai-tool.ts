// Vercel AI SDK: a first job with no account.  npm i ai zod lamdis @ai-sdk/openai
import { generateText, tool } from "ai";
import { z } from "zod";
import { openai } from "@ai-sdk/openai";
import { Lamdis } from "lamdis";

const exchange = new Lamdis(); // anonymous: no key, no balance, no sign-in

export const observeWorld = tool({
  description:
    "Pay somebody to go and photograph whether `predicate` is true at `where`. " +
    "fee_minor is cents, paid for honest evidence either way. Nothing is charged until " +
    "there is proof. Returns pay_at (give it to the person) and token (keep it).",
  parameters: z.object({
    predicate: z.string(),
    where: z.string(),
    lat: z.number(),
    lon: z.number(),
    fee_minor: z.number().int().positive(),
  }),
  execute: async ({ predicate, where, lat, lon, fee_minor }) => {
    const posted = await exchange.observe({ predicate, where, lat, lon, radius_m: 150, fee_minor });
    return { job: posted.job, status: posted.status, pay_at: posted.payAt, token: posted.token };
  },
});

export const jobStatus = tool({
  description: "Where a job has got to. Pass the token that came back when it was posted.",
  parameters: z.object({ job: z.string(), token: z.string() }),
  execute: async ({ job, token }) => exchange.job(job, token).status(),
});

const { text } = await generateText({
  model: openai("gpt-4o"), // any provider the AI SDK supports
  tools: { observeWorld, jobStatus },
  maxSteps: 3,
  prompt: "Is the 'For Lease' sign still up at 1200 Valencia St, San Francisco (37.7527, -122.4207)? Pay up to $8.",
});
console.log(text); // send the person the pay link
