# Working Agreement

Use small, independently testable iterations as internal steps toward the complete requested goal. The user provides requirements and final manual acceptance; Codex owns proposals, implementation, and self-verification.

## Global Constraints

Work in the current branch and checkout by default. These constraints apply project-wide, including actions performed through tools, scripts, or sub-agents.

- **Git branches:** Never create a Git branch unless the user's prompt explicitly requests branch creation.
- **Git worktrees:** Never create a Git worktree unless the user's prompt explicitly requests worktree creation.

Authorization for one action does not authorize the other. General implementation, task-isolation, or parallel-work requests do not authorize either action.

### Continuous Goal Execution

- **Full scope:** Carry the entire user-requested goal through implementation and self-verification. Internal phases, milestones, and iterations do not authorize partial delivery or deferring remaining requirements. Only the user may reduce the requested scope.
- **Continuous execution:** While authorized work remains actionable, continue directly to the next implementation, integration, documentation, or verification step. Do not ask the user to say "continue" for work already requested.
- **Progress without ending:** Use intermediate commentary for progress, partial results, and remaining work. Never voluntarily issue a final response or end the turn at an unfinished milestone, even when the goal runner would automatically restart execution. Automatic continuation is not a substitute for completing the goal before returning.
- **Final-response gate:** Before final delivery, check every requirement in the full requested scope against implementation and verification evidence. All required work and self-checks must be complete, failures resolved, and delivery materials ready. A subset of passing tests or a completed phase does not satisfy this gate. Manual user acceptance remains a separate step and must not be claimed in advance.
- **Questions and blockers:** Ask necessary questions through an available in-progress input mechanism, keep dependent work pending, and continue unaffected work. Diagnose and recover from ordinary failures. A blocker report must identify the actual missing input or external dependency and must not present an unfinished goal as delivered.
- **Interruptions:** Respect an explicit user pause or cancellation and mandatory platform limits. These are interruptions, not successful completion. Context compaction is not a reason to stop; after recovery, retain the original goal, completed work, and remaining requirements and continue. Never deliberately end a turn to trigger automatic restart or to turn a partial milestone into a handoff.

## Workflow

1. **Clarify:** Establish the goal, use cases, and scope from project context. Clarify gaps affecting business rules or acceptance; continue unaffected work.
2. **Propose:** Present the proposal below before coding. Keep simple proposals in chat; document complex designs in the repository.
3. **Implement:** Proceed once the proposal is clear, unless the user requests proposal review first. Resolve blocking questions before dependent work.
4. **Verify and deliver:** Run relevant checks and fix failures. Provide completed changes, startup/access instructions, check results, unverified items, known limitations, and manual acceptance steps with expected outcomes.
5. **Iterate:** Fix unmet acceptance criteria in the current iteration; scope new requirements separately. Update scope and acceptance criteria when requirements change; leave business tradeoffs to the user.

Track status as **In progress → Awaiting acceptance → Accepted**. Move to Awaiting acceptance after self-verification passes, and to Accepted only after explicit user confirmation. Record verification results separately from user acceptance.

## Proposal Contents

- **What:** Target behavior, scope, and deliverables.
- **How:** Key design, relevant component interactions and data flows, and implementation steps.
- **Technology:** Name the relevant languages, frameworks, storage systems, and tools. Explain why each is chosen and exactly where and how it is used in the design.
- **Acceptance:** Observable criteria the user can verify manually.
