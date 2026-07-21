# SEO Strategy — Askolo

## App summary
Askolo is an AI-powered personal and family assistant SaaS app: habits, goals, daily plans, calendar, chores, notes, action items, and AI coaching. Optional Google Calendar and Gmail integrations.

## Rendering architecture
**React SPA (Vite + Wouter).** All routing is client-side. There is no SSR or SSG layer. Googlebot can eventually render JS; social preview bots and AI crawlers cannot.

## Public-facing pages (in scope for SEO)
- `/` — Landing page (primary SEO surface)
- `/privacy` — Privacy Policy
- `/terms` — Terms of Service
- `/sign-in`, `/sign-up` — Clerk auth pages (low SEO value; index, follow is fine)

## Authenticated / app pages (out of scope)
- `/dashboard`, `/habits`, `/goals`, `/plan`, `/calendar`, `/chores`, `/notes`, `/actions`, `/assistant`, `/email`
  All protected by Clerk auth; SPA rendering is acceptable here.

## Target audience
Individuals and families looking for an all-in-one productivity and life-management tool with AI coaching.

## Primary keywords (inferred)
- personal assistant app
- habit tracker
- AI life planner
- family task manager
- daily planner app

## Dismissed categories
- (None yet)
