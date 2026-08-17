# Public Release and Ownership Transfer Checklist

## Purpose

Use this checklist before transferring Capcom to a personal account and promoting
the repository publicly. A repository transfer changes its GitHub owner; it does
not by itself establish intellectual-property ownership or remove historical Git
content.

## Current Preparation Status

- The repository is already public.
- Current tracked files do not contain developer-specific workstation paths.
- Current tracked files use Gantry consistently and do not refer to its former
  local checkout name.
- Repository links in the README are relative and remain valid after transfer.
- Real local environment files are ignored. Only `.env.example` templates are
  tracked.
- A history-wide rule-based scan found no committed API keys, private keys, JWTs,
  credential files, or real `.env` files.
- Historical commits retain the previous path text and company-email author
  metadata. A normal GitHub transfer preserves that history.

## Approval Gate

Before transfer, obtain written confirmation from the current owner that:

- Capcom may be transferred to the intended personal account.
- The source code and approved documentation may be published and promoted.
- The selected open-source license is authorized.
- The project name, screenshots, architecture, and product material may be used
  publicly.

Do not describe the project as independently owned or open source until these
points are confirmed.

## Content Review

Review the following material explicitly rather than relying only on automated
secret scanning:

- Product strategy, market research, decision records, and archived planning
  documents under `docs/`.
- Runtime integration notes and screenshots for internal hostnames, account names,
  infrastructure details, or non-public runtime behavior.
- Git commit author and committer metadata.
- Issues, pull requests, Actions logs, releases, packages, wikis, and GitHub Pages.
- Third-party dependencies, copied assets, fixtures, and their licenses.

Remove or rewrite anything that is not approved for public release. Use fictional
domains and identifiers in examples. Repository documentation must use relative
paths or placeholders such as `<path-to-capcom>` and `<path-to-gantry>`.

## Security Verification

Immediately before transfer:

1. Fetch all branches and tags.
2. Run a maintained secret scanner across the complete Git history and current
   working tree with findings redacted.
3. Confirm that `.env`, `.env.local`, credentials, certificates, database dumps,
   logs, and generated runtime state are ignored and untracked.
4. Review repository Actions secrets, deploy keys, webhooks, environments, and
   package credentials.
5. Revoke and rotate any credential found in Git history before attempting history
   cleanup.

Never include secret values in an audit report, issue, pull request, or commit.

## Repository Preparation

Before initiating the transfer:

1. Merge or close the branches and pull requests that should not remain active.
2. Confirm the default branch and branch-protection rules.
3. Add the approved `LICENSE` file and corresponding README notice.
4. Add a concise project-status and non-affiliation statement if required by the
   ownership approval.
5. Verify that a fresh clone builds and runs using only documented prerequisites
   and example environment files.
6. Record the source owner, destination owner, repository name, approval reference,
   and transfer date outside the repository.

## GitHub Transfer

An authorized repository administrator should:

1. Open the repository settings on GitHub.
2. Review the transfer warnings and destination-account requirements.
3. Transfer the repository to the approved personal account.
4. Have the destination owner accept the transfer when GitHub requests
   confirmation.
5. Verify repository visibility, collaborators, teams, branch protection,
   environments, Actions permissions, webhooks, deploy keys, packages, Pages, and
   security settings after transfer.

Existing clones normally redirect through GitHub, but contributors should update
their `origin` remote to the new canonical URL.

## Post-Transfer Verification

- Clone the transferred repository into a clean directory.
- Repeat the secret and content scans against the transferred repository.
- Run the documented build and test commands.
- Check every README and documentation link from GitHub's rendered view.
- Confirm that the license and ownership language are visible and accurate.
- Enable appropriate secret scanning and dependency alerts on the destination
  repository.
- Only then publish external announcements or a LinkedIn post.

## History Rewrite Decision

A normal transfer preserves commit hashes, branches, tags, author metadata, and old
file contents. If the approved public repository must not retain historical
workstation paths or company-email metadata, plan a separate coordinated history
rewrite before promotion. History rewriting changes commit hashes and requires
review of pull requests, forks, collaborators' clones, tags, and cached references;
it must not be performed as an incidental cleanup step.
