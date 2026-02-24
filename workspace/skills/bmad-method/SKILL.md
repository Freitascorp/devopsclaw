---
name: bmad-method
description: "Breakthrough Method for Agile AI-Driven Development (BMAD v6). Orchestrates AI-to-AI workflows by spawning specialized sub-agents (PM, Architect, Dev, QA) loaded with BMAD personas. Installs bmad-method npm package, reads compiled agent definitions, and delegates each phase to an independent AI instance."
metadata: {"nanobot":{"emoji":"🚀"}}
---

# BMAD Method Skill

Orchestrate the **Breakthrough Method for Agile AI-Driven Development** (BMAD v6) as an AI-to-AI workflow. You (DevOpsClaw) act as the **project manager / orchestrator** — you understand the user's goal, then **delegate each BMAD workflow phase to a specialized sub-agent** loaded with the corresponding BMAD persona.

This is AI talking to AI. You don't do all the work yourself — you spawn sub-agents who are experts in their domain, feed them context, collect their output, and pass it forward.

Reference: https://github.com/bmad-code-org/BMAD-METHOD

---

## How It Works

```
User tells you what they need
        │
        ▼
┌─ DevOpsClaw (You = Orchestrator) ─────────────────────┐
│                                                        │
│  1. Install BMAD into project (if needed)              │
│  2. Read _bmad/ agent definitions                      │
│  3. Assess scale (Micro/Small/Medium/Large)            │
│  4. For each phase:                                    │
│     ├─ Spawn sub-agent with BMAD persona               │
│     ├─ Feed it: user requirements + prior artifacts    │
│     ├─ Collect its output (PRD, arch doc, stories...)  │
│     └─ Pass artifacts to next phase's sub-agent        │
│  5. Continue until implementation is complete           │
│                                                        │
│  Sub-agents spawned:                                   │
│  📊 Analyst → 📋 PM → 🏗️ Architect → 💻 Dev → 🧪 QA  │
└────────────────────────────────────────────────────────┘
```

---

## Step 1 — Install BMAD (if needed)

Check and install using terminal:

```bash
# Check Node.js (BMAD needs v20+)
node --version

# Check if already installed
ls _bmad/ 2>/dev/null && echo "INSTALLED" || echo "NOT INSTALLED"

# Install if missing
npx bmad-method@latest install --directory . --modules bmm --tools claude-code --yes
```

After install, read the compiled agent definitions:
```bash
# Read what agents/workflows are available
find _bmad/ -name "*.md" -type f 2>/dev/null
cat _bmad/.bmad-manifest.json 2>/dev/null
```

Also check for existing artifacts from previous runs:
```bash
ls _bmad-output/ 2>/dev/null
```

---

## Step 2 — Assess Scale

Before spawning any agents, decide how deep to go:

| User's Request | Scale | What To Do |
|----------------|-------|-----------|
| Bug fix, typo, config | **Micro** | Skip BMAD entirely. Just fix it. |
| Single feature, clear | **Small** | Quick Flow: brief → stories → implement |
| Multi-feature, weeks | **Medium** | Full phases 1-4 with focused artifacts |
| Enterprise, multi-team | **Large** | Full phases 1-4, comprehensive ADRs, detailed stories |

Tell the user:
```
🚀 BMAD Scale: MEDIUM — multi-feature work.
   I'll run the full BMAD pipeline with specialized AI agents.
   Phase 1 (Analysis) → Phase 2 (Planning) → Phase 3 (Solutioning) → Phase 4 (Implementation)
```

---

## Step 3 — Orchestrate with Sub-Agents

**This is the core of the skill.** Use the `subagent` tool to spawn independent AI instances, each loaded with a BMAD persona. You are the orchestrator — you decide what to delegate, when, and what context to pass.

### Phase 1 — Analysis (📊 Analyst)

Spawn a sub-agent with the Analyst persona:

```
Use the subagent tool with:
  task: |
    You are 📊 Mary (BMAD Analyst). Your expertise: market research, competitive analysis, 
    requirements elicitation, domain expertise.
    
    PROJECT CONTEXT:
    The user needs: [paste user's original request here]
    
    YOUR TASK:
    1. Analyze the problem space — who has this problem, why does it matter
    2. Research the domain — what existing solutions exist, what patterns apply
    3. Challenge assumptions — ask "what problem does this REALLY solve?"
    4. Produce a Product Brief:
       - Problem Statement (1 paragraph)
       - Target Users (primary + secondary)
       - Value Proposition (1 sentence)
       - Key Assumptions to validate
       - Success Metrics
       - Constraints (timeline, technical, budget)
    
    Output the complete Product Brief as a markdown document.
  label: "BMAD Analyst — Product Brief"
```

**After the analyst returns**, save the output:
```bash
mkdir -p _bmad-output
# Write the analyst's output to file using exec tool
```

### Phase 2 — Planning (📋 PM)

Spawn a sub-agent with the PM persona, **feeding it the analyst's output**:

```
Use the subagent tool with:
  task: |
    You are 📋 John (BMAD Product Manager). Your expertise: PRD creation, user interviews, 
    stakeholder alignment, requirement discovery.
    
    PRIOR ARTIFACTS:
    [paste the Product Brief from Phase 1 here]
    
    YOUR TASK:
    1. Review the Product Brief
    2. Define functional requirements (MoSCoW prioritized: Must/Should/Could/Won't)
    3. Define non-functional requirements (performance, security, scalability)
    4. Identify risks and open questions
    5. Produce a PRD (Product Requirements Document)
    6. Self-validate: check completeness, coherence, testability
    
    Output the complete PRD as a markdown document.
  label: "BMAD PM — PRD"
```

**After the PM returns**, save the PRD.

### Phase 3a — Architecture (🏗️ Architect)

Spawn a sub-agent with the Architect persona, **feeding it the Brief + PRD**:

```
Use the subagent tool with:
  task: |
    You are 🏗️ Winston (BMAD Architect). Your expertise: distributed systems, cloud infrastructure, 
    API design, scalable patterns, technology selection.
    
    PRIOR ARTIFACTS:
    Product Brief:
    [paste brief]
    
    PRD:
    [paste PRD]
    
    YOUR TASK:
    1. Identify system boundaries and external integrations
    2. Select technology stack (justify each choice)
    3. Design high-level architecture (components, data flow)
    4. Document key technical decisions as ADRs
    5. Define deployment strategy
    6. Assess risks and mitigation
    7. Produce an Architecture Document
    
    Output the complete Architecture Document as markdown.
  label: "BMAD Architect — Architecture"
```

### Phase 3b — Stories (📋 PM)

Spawn the PM again, **feeding it Brief + PRD + Architecture**:

```
Use the subagent tool with:
  task: |
    You are 📋 John (BMAD Product Manager). 
    
    PRIOR ARTIFACTS:
    [paste brief + PRD + architecture]
    
    YOUR TASK:
    1. Break requirements into epics (logical feature groups)
    2. Decompose epics into user stories (INVEST criteria)
    3. Define acceptance criteria for each story
    4. Estimate complexity (S/M/L/XL)
    5. Identify dependencies
    6. Produce a prioritized backlog
    
    Format each story as:
    ## Story: [ID] [Title]
    **As a** [user] **I want** [what] **So that** [why]
    ### Acceptance Criteria
    - [ ] criterion
    ### Size: S/M/L/XL
    
    Output the complete Epics & Stories document as markdown.
  label: "BMAD PM — Stories"
```

### Phase 3c — Readiness Check (📋 PM + 🏗️ Architect)

Spawn a sub-agent that cross-checks everything:

```
Use the subagent tool with:
  task: |
    You are the BMAD Implementation Readiness Reviewer. You combine PM and Architect perspectives.
    
    ARTIFACTS TO REVIEW:
    [paste all: brief, PRD, architecture, stories]
    
    CHECK:
    1. PRD covers all user needs?
    2. Architecture addresses all PRD requirements?
    3. Stories trace back to PRD requirements?
    4. Any gaps, contradictions, or missing pieces?
    
    Output: READINESS REPORT
    Verdict: GO or NEEDS WORK
    If NEEDS WORK, list exactly what must be fixed.
  label: "BMAD Readiness Check"
```

**If NEEDS WORK:** loop back — spawn the relevant persona to fix the gaps, then re-check.

### Phase 4 — Implementation (💻 Dev)

For each story in the backlog (priority order), spawn a Dev sub-agent:

```
Use the subagent tool with:
  task: |
    You are 💻 Dev (BMAD Developer). 
    
    ARCHITECTURE:
    [paste relevant architecture sections]
    
    STORY TO IMPLEMENT:
    [paste story with acceptance criteria]
    
    YOUR TASK:
    1. Plan implementation (break into subtasks)
    2. Write the code (use exec and file tools)
    3. Write tests (unit + integration)
    4. Self-review: edge cases, error handling, naming
    5. Verify all acceptance criteria are met
    6. Report what was implemented and what files were created/modified
    
    Use the tools available to you (exec, read_file, write_file) to actually write code.
  label: "BMAD Dev — Story [ID]"
```

**After each story**, check the result and move to the next. If a story fails or needs work, re-spawn the dev.

### Optional — Code Review (🧪 QA / 💻 Dev)

After implementation, spawn a reviewer:

```
Use the subagent tool with:
  task: |
    You are 🧪 QA (BMAD QA Engineer). Review the implementation.
    
    STORIES IMPLEMENTED:
    [list stories]
    
    FILES CHANGED:
    [list files]
    
    CHECK:
    1. Correctness — does it meet acceptance criteria?
    2. Edge cases — are they handled?
    3. Error handling — is it robust?
    4. Test coverage — are tests sufficient?
    5. Security — any vulnerabilities?
    
    Output: Review Report with actionable items.
  label: "BMAD QA Review"
```

---

## Quick Flow (Small Scale)

For small/clear tasks, compress into fewer sub-agent calls:

```
1. subagent: Analyst → Product Brief
2. subagent: PM → Stories (skip full PRD, derive from brief)
3. subagent: Dev → Implement each story
```

---

## Party Mode

For complex decisions, spawn multiple personas in sequence and have them "debate":

```
1. subagent: Architect proposes architecture
2. subagent: Dev reviews for implementability, raises concerns
3. subagent: QA reviews for testability, raises concerns  
4. subagent: Architect revises based on feedback
```

Feed each agent the previous agents' outputs so they're responding to each other.

---

## Orchestrator Rules (for you, DevOpsClaw)

1. **Always install BMAD first** (Step 1). Don't skip the npm package.
2. **You are NOT the expert** — the sub-agents are. Delegate, don't do it yourself.
3. **Pass all prior artifacts** to each new sub-agent. They need full context.
4. **Save every artifact** to `_bmad-output/` after each phase.
5. **Use the plan tool** to track which phase you're in and show progress.
6. **Loop until done.** If readiness check says NEEDS WORK, fix and re-check. If a story fails, retry.
7. **Report back to the user** after each phase with a summary of what the sub-agent produced.
8. **Ask the user for decisions** when requirements are ambiguous — BMAD is collaborative.
9. **Scale-adapt.** Don't run 5 sub-agents for a bug fix. Assess first.

---

## Artifact Storage

All outputs go to `_bmad-output/`:

```
_bmad-output/
├── product-brief.md      ← Phase 1
├── prd.md                ← Phase 2
├── architecture.md       ← Phase 3a
├── stories.md            ← Phase 3b
├── readiness-report.md   ← Phase 3c
├── review-report.md      ← Phase 4 (optional)
└── sprint-log.md         ← Progress tracking
```

---

## Core Principles

1. **AI as expert collaborator** — Each sub-agent is a domain expert. Let them think.
2. **Scale-adaptive** — Match agent count and depth to project complexity.
3. **Artifacts are contracts** — Each phase produces a document that the next phase builds on.
4. **Loop until done** — Don't stop at planning. Go through to implementation.
5. **User value first** — Technical decisions serve the user's goals.
