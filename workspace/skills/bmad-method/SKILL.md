---
name: bmad-method
description: "Run BMAD quick-flow-solo-dev via Gemini CLI. Fully autonomous — never pauses, never asks user, completes every task end to end."
metadata: {"nanobot":{"emoji":"🚀"}}
---

# BMAD Method Skill

Run BMAD development workflows via the `gemini` tool. BMAD is already installed.

## ⚠️ PRIME DIRECTIVE — READ THIS FIRST

**You MUST NOT stop, pause, summarize-and-wait, or ask the user ANYTHING during this workflow.**

When gemini returns output — no matter what it says — you IMMEDIATELY call the gemini tool again to continue. You do not:
- Ask "should I continue?"
- Ask "do you want me to proceed?"
- Summarize and wait for input
- Relay Gemini's questions to the user
- Pause for any reason

If Gemini asks a question → you answer it yourself and call gemini again.
If Gemini says "should I proceed?" → you call gemini with "Yes, proceed."
If Gemini completed a task → you call gemini with "Continue to the next task."
If Gemini asks for input → you provide it from the user's vision and call gemini again.

**The ONLY time you stop is when ALL tasks are 100% complete.**

---

## What to do

1. Call the `gemini` tool with `working_dir` set to the project root
2. In the prompt, include:
   - Run `/bmad-agent-bmm-quick-flow-solo-dev`
   - The user's full app vision (paste verbatim)
   - This exact instruction to Gemini: **"Complete every single task in this workflow without stopping. Do not ask if you should proceed — just proceed. Do not ask for additional input — use the app vision above to answer everything. When a task is done, immediately start the next one. Do not stop until every task is finished."**
3. When gemini returns → **immediately call gemini again** with "Continue. Complete all remaining tasks." Include the app vision again.
4. Repeat step 3 until gemini reports that ALL tasks are done.

---

## After each gemini call

1. Read the output
2. **Call gemini again immediately.** Prompt: "Continue from where you left off. Complete all remaining tasks without stopping. App vision: [paste vision]"
3. Do NOT ask the user anything. Do NOT summarize and wait. Do NOT relay questions.
4. Only after ALL tasks are done: archive artifacts to `bmad-projects/<project-name>/`, clean `_bmad-output/`, then give the user a final summary with the archive location.

---

## How to answer on the user's behalf

Everything comes from the app vision. You never need to ask:

- App description → from vision
- Target users → infer from vision
- Tech stack → what makes sense for the app, or what vision specifies
- Features → derive from vision
- Priorities → core features first
- Any ambiguity → pick what best serves the vision

---

## Navigating BMAD menus and elicitations

- Menu → pick the best option for the project
- Elicitations → always opt in, answer from vision
- "Want to continue?" → always yes
- "Anything else?" → continue until exhausted
- Checklist/validation → confirm passing items, fix failing ones

---

## When the workflow is COMPLETE — Archive & Clean

Once ALL BMAD tasks are finished and all artifacts have been generated:

1. **Derive a project name** from the app vision (kebab-case, e.g. `music-streaming-app`, `portfolio-dashboard`)
2. **Create the archive folder**: `bmad-projects/<project-name>/` inside the workspace root
3. **Copy the entire `_bmad-output/` contents** into that archive folder:
   ```
   bmad-projects/<project-name>/implementation-artifacts/
   bmad-projects/<project-name>/planning-artifacts/
   bmad-projects/<project-name>/test-artifacts/
   ```
4. **Clean `_bmad-output/`** — remove all generated files and subdirectories inside it so it's empty and ready for the next BMAD development cycle. Keep the `_bmad-output/` directory itself.
5. **Tell the user** the artifacts have been archived to `bmad-projects/<project-name>/` and that `_bmad-output/` has been cleaned.

Use the `exec` tool to run shell commands like `cp -r` and `rm -rf` for this. Do NOT ask the user — just do it.

---

## Rules

1. **NEVER STOP. NEVER ASK. NEVER WAIT.** Always call gemini again immediately.
2. **Always set working_dir** to the project root
3. **Include the app vision** in every gemini call
4. **Tell Gemini to never stop** — include the no-stopping instruction in every prompt
5. **Complete every task** — don't stop until the entire workflow is done
6. **Archive artifacts and clean `_bmad-output/`** when the workflow is done (see above)
7. **Only give the user a summary at the very end** — include the archive location
