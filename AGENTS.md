# Working Agreement

Use small, independently testable iterations focused on current needs. The user provides requirements and final manual acceptance; Codex owns proposals, implementation, and self-verification.

## Global Constraints

Work in the current branch and checkout by default. These constraints apply project-wide, including actions performed through tools, scripts, or sub-agents.

- **Git branches:** Never create a Git branch unless the user's prompt explicitly requests branch creation.
- **Git worktrees:** Never create a Git worktree unless the user's prompt explicitly requests worktree creation.

Authorization for one action does not authorize the other. General implementation, task-isolation, or parallel-work requests do not authorize either action.

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
