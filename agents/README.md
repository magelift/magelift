# Shop skills

MageLift bundles these skills for agents working in a Magento shop.

| Skill | Use it for |
| --- | --- |
| `magelift-dependencies` | Check PHP, Docker, and cloud tools |
| `magelift-local-runtime` | Start the shop with Docker Compose |
| `magelift-configure` | Edit and validate `magelift.php` |
| `magelift-migrate` | Import settings, then rehearse a database and media transfer |
| `magelift-operate` | Check a running shop and deploy changes |

The MageLift repository has its own [contributor skills](../.agents/skills/).

## Install in a shop

Run these commands from the shop checkout. The direct installer uses the
copies built into the CLI.

```sh
magelift skills list
magelift skills install --agent codex --scope project --mode symlink
magelift skills verify --agent codex --scope project
```

Select Claude Code with `--agent claude-code` or use `--skill magelift-operate`
to install one skill. For a global copy, pass `--scope global --mode copy`.
The installer asks for `--replace` before replacing an unrelated destination.

From a MageLift source checkout, the optional skills CLI backend can manage
installation.

```sh
magelift skills install --backend skills-cli --agent codex --scope project
```

For another agent, check its skill path and copy or link the relevant folder
from [`agents/skills/`](skills/).
