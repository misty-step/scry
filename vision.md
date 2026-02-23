# Vision

## One-Liner
The most powerful, simple, frictionless memory management and learning system — an AI-enhanced Anki killer with an agentic UX.

## North Star
Open Scry on your phone. The most needed review item appears instantly. Answer it, see the next. Generate new content from anything — text, images, audio, web pages — with zero friction. An autonomous tutor that knows what you know, what you struggle with, and what to show you next.

## Key Differentiators
- **Quizzes over flashcards** — objective assessment (right/wrong), no self-reporting
- **Concept-centric model** — multiple phrasings test the same concept, updating shared memory statistics
- **Agentic content generation** — multi-modal inputs (text, images, video, audio, files) processed by LLM pipeline into quiz items
- **Autonomous tutoring** — leverages known content, interaction history, strengths/struggles to dynamically prioritize reviews beyond standard spaced repetition
- **Free-response AI grading** — multiple choice, true/false, and free-response with LLM-powered assessment
- **Generative UI** — agentic workflow with generative interfaces (Vercel AI SDK, RenderJSON patterns)

## Target User
Anyone who learns systematically — students, professionals, lifelong learners. People who find Anki powerful but painful. Those who want the benefits of spaced repetition without the manual card-creation grind.

## Architecture Direction
- Agentic workflows via Claude Agent SDK or OpenAI Agent Kit (evaluate both; draw from Moneta and Volume sibling repos)
- Expanded input pipeline: browser extension, API polling/listeners, batch processing
- Intelligent filtering (ads, irrelevant material)
- Cognitive science foundations: desirable difficulty, optimal scheduling, proven methodologies

## Current Focus
Ship a working product that proves the core loop: multi-modal input -> AI quiz generation -> concept-centric review -> objective assessment. Then layer on agentic automation.

---
*Last updated: 2026-02-16*
*Updated during: /groom session*
