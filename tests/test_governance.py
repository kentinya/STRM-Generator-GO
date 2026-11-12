import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import check_governance as governance


class GovernanceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        files = {
            ".go-version": "1.27.2\n",
            "go.mod": "module test-fixture\n\ngo 1.27.2\n",
            "README.md": "[architecture](docs/architecture.md)\n",
            "docs/architecture.md": "# test architecture\n",
            "docs/requirements.md": "# test requirements\n",
            "docs/roadmap.md": "| 1 | 安全单轮生成 | PLANNED |\n",
            "SLICE.md": "Slice ID: 1\nStatus: PLANNED\nBase SHA: NOT SET\nImplementation Head: NOT SET\n\n## Required Outcomes\nA safe result.\n\n## Closure Packet\nNOT STARTED\n",
            "TASK.md": "# NO ACTIVE IMPLEMENTATION TASK\n",
        }
        spec = {"go": {"version": "1.27.2"}, "required_files": [*files, "dev/environment-spec.json"]}
        files["dev/environment-spec.json"] = json.dumps(spec)
        for name, contents in files.items():
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(contents)

    def edit(self, name, before, after):
        path = self.root / name
        path.write_text(path.read_text().replace(before, after))

    def git(self, *args):
        result = subprocess.run(["git", "-C", str(self.root), *args], check=True,
                                text=True, capture_output=True)
        return result.stdout.strip()

    def commit(self):
        # Only this disposable test repository uses a synthetic identity.
        self.git("add", "--all")
        self.git("-c", "user.name=Governance Test", "-c", "user.email=test@example.invalid",
                 "commit", "-m", "test fixture")
        return self.git("rev-parse", "HEAD")

    def test_current_bootstrap_is_valid(self):
        self.assertIn("preparation", governance.validate(self.root, True))

    def test_local_reference_is_optional_and_not_pinned(self):
        plan = self.root / "STRM Generator Go.md"
        for content in (None, "# Local reference V9.9\n", "arbitrary local notes\n"):
            with self.subTest(content=content):
                if content is not None:
                    plan.write_text(content)
                self.assertIn("preparation", governance.validate(self.root, True))

    def test_missing_document_is_rejected(self):
        (self.root / "docs/requirements.md").unlink()
        with self.assertRaisesRegex(ValueError, "Required file"):
            governance.validate(self.root, True)

    def test_broken_link_is_rejected(self):
        self.edit("README.md", "(docs/architecture.md)", "(docs/nonexistent.md)")
        with self.assertRaisesRegex(ValueError, "Markdown link"):
            governance.validate(self.root, True)

    def test_roadmap_mismatch_is_rejected(self):
        self.edit("docs/roadmap.md", "| 1 | 安全单轮生成 | PLANNED |", "| 1 | 安全单轮生成 | ACTIVE |")
        with self.assertRaisesRegex(ValueError, "Roadmap"):
            governance.validate(self.root, True)

    def test_uncommitted_activation_cannot_pass(self):
        self.git("init", "-b", "main")
        self.commit()
        self.edit("SLICE.md", "Status: PLANNED", "Status: ACTIVE")
        self.edit("docs/roadmap.md", "| 1 | 安全单轮生成 | PLANNED |", "| 1 | 安全单轮生成 | ACTIVE |")
        with self.assertRaisesRegex(ValueError, "committed in HEAD"):
            governance.validate(self.root)
        with self.assertRaisesRegex(ValueError, "Bootstrap"):
            governance.validate(self.root, True)

    def test_no_head_is_not_normal_governance(self):
        # Fixtures may live under artifacts/; never discover the parent HEAD.
        self.git("init", "-b", "main")
        with self.assertRaisesRegex(ValueError, "Git boundary"):
            governance.validate(self.root)

    def test_committed_planning_is_valid_but_not_active(self):
        self.git("init", "-b", "main")
        self.commit()
        self.assertIn("no active implementation", governance.validate(self.root))

    def history_fixture(self):
        self.git("init", "-b", "main")
        self.git("remote", "add", "origin", "https://github.com/test/docs.git")
        (self.root / "docs/old.md").write_text("# Original evidence\n")
        self.edit("SLICE.md", "A safe result.", "A safe result. [evidence](docs/old.md#original-evidence)")
        self.edit("docs/roadmap.md", "| 1 |", "[evidence](old.md#original-evidence)\n| 1 |")
        revision = self.commit()
        url = f"https://github.com/test/docs/blob/{revision}/docs/old.md#original-evidence"
        return revision, url

    def test_pinned_history_link_migration_survives_commit(self):
        _, url = self.history_fixture()
        self.edit("SLICE.md", "docs/old.md#original-evidence", url)
        self.edit("docs/roadmap.md", "old.md#original-evidence", url)
        (self.root / "docs/old.md").unlink()
        self.assertIn("planning", governance.validate(self.root))
        self.commit()
        self.assertIn("planning", governance.validate(self.root))

    def test_history_migration_does_not_allow_contract_changes(self):
        _, url = self.history_fixture()
        for replacement in (url.replace("test/docs", "other/docs"),
                            url.replace("#original-evidence", "#different"),
                            url.replace("/blob/", "/blob/main/")):
            with self.subTest(url=replacement):
                self.edit("SLICE.md", "docs/old.md#original-evidence", replacement)
                with self.assertRaises(ValueError):
                    governance.validate(self.root)
                self.edit("SLICE.md", replacement, "docs/old.md#original-evidence")
        self.edit("SLICE.md", "docs/old.md#original-evidence", url)
        self.edit("SLICE.md", "A safe result.", "A changed obligation.")
        with self.assertRaisesRegex(ValueError, "Working Slice contract"):
            governance.validate(self.root)

    def test_history_migration_rejects_different_blob(self):
        _, url = self.history_fixture()
        (self.root / "docs/old.md").write_text("# Changed evidence\n")
        self.commit()
        self.edit("SLICE.md", "docs/old.md#original-evidence", url)
        with self.assertRaisesRegex(ValueError, "content differs"):
            governance.validate(self.root)

    def test_history_migration_requires_ancestor_revision(self):
        _, url = self.history_fixture()
        self.git("switch", "-c", "unrelated")
        (self.root / "branch-only.txt").write_text("Unmerged branch\n")
        other = self.commit()
        self.git("switch", "main")
        replacement = url.replace(url.split("/blob/")[1].split("/")[0], other)
        self.edit("SLICE.md", "docs/old.md#original-evidence", replacement)
        with self.assertRaisesRegex(ValueError, "Git boundary"):
            governance.validate(self.root)

    def test_real_activation_requires_real_base(self):
        self.git("init", "-b", "main")
        base = self.commit()
        self.edit("SLICE.md", "Status: PLANNED", "Status: ACTIVE")
        self.edit("SLICE.md", "Base SHA: NOT SET", "Base SHA: " + base)
        self.edit("docs/roadmap.md", "| 1 | 安全单轮生成 | PLANNED |", "| 1 | 安全单轮生成 | ACTIVE |")
        self.commit()
        self.assertIn("committed contract", governance.validate(self.root))
        self.edit("SLICE.md", "Base SHA: " + base, "Base SHA: " + "0" * 40)
        self.commit()
        with self.assertRaisesRegex(ValueError, "Git boundary"):
            governance.validate(self.root)

    def test_bootstrap_cannot_admit_task(self):
        (self.root / "TASK.md").write_text("# Task 1.1\nParent Slice: 1\nStatus: PLANNED\n")
        with self.assertRaisesRegex(ValueError, "Bootstrap"):
            governance.validate(self.root, True)

    def activate_with_task(self):
        self.git("init", "-b", "main")
        base = self.commit()
        self.edit("SLICE.md", "Status: PLANNED", "Status: ACTIVE")
        self.edit("SLICE.md", "Base SHA: NOT SET", "Base SHA: " + base)
        self.edit("docs/roadmap.md", "| 1 | 安全单轮生成 | PLANNED |", "| 1 | 安全单轮生成 | ACTIVE |")
        checkpoint = self.commit()
        (self.root / "TASK.md").write_text(f"# Task 1.1\nTask ID: 1.1\nParent Slice: 1\nStatus: IN PROGRESS\nTask Base: {base}\nContract Checkpoint: {checkpoint}\nTest Level: T4\nDifficulty: High\n")
        return checkpoint

    def test_valid_task_allows_implementation_head_reporting(self):
        checkpoint = self.activate_with_task()
        self.edit("SLICE.md", "Implementation Head: NOT SET", "Implementation Head: " + checkpoint)
        self.assertIn("committed contract", governance.validate(self.root))

    def test_task_wrong_parent_is_rejected(self):
        self.activate_with_task()
        self.edit("TASK.md", "Parent Slice: 1", "Parent Slice: 2")
        with self.assertRaisesRegex(ValueError, "parent mismatch"):
            governance.validate(self.root)

    def test_final_review_can_retain_passed_task(self):
        checkpoint = self.activate_with_task()
        self.edit("TASK.md", "Status: IN PROGRESS", "Status: PASS")
        self.edit("SLICE.md", "Status: ACTIVE", "Status: READY FOR A REVIEW")
        self.edit("SLICE.md", "Implementation Head: NOT SET", "Implementation Head: " + checkpoint)
        self.edit("docs/roadmap.md", "| ACTIVE |", "| READY FOR A REVIEW |")
        self.commit()
        self.assertIn("committed contract", governance.validate(self.root))

    def test_changed_outcome_cannot_reuse_old_task_checkpoint(self):
        self.activate_with_task()
        self.edit("SLICE.md", "A safe result.", "A changed obligation.")
        self.commit()
        with self.assertRaisesRegex(ValueError, "Task checkpoint"):
            governance.validate(self.root)

    def standalone_fixture(self, activate=True):
        implementation = self.activate_with_task()
        self.edit("SLICE.md", "Status: ACTIVE", "Status: PASS / CLOSED")
        self.edit("SLICE.md", "Implementation Head: NOT SET", "Implementation Head: " + implementation)
        self.edit("SLICE.md", "NOT STARTED", "Decision: PASS")
        self.edit("docs/roadmap.md", "| ACTIVE |", "| PASS / CLOSED |")
        (self.root / "TASK.md").write_text("# NO ACTIVE IMPLEMENTATION TASK\n")
        base = self.commit()
        plan = (
            "# NO ACTIVE IMPLEMENTATION TASK\n\n"
            "Task ID: FIX-TG-01\nTask Type: standalone-fix\n"
            "Parent Slice: NONE\nRelated Slice: 1\nStatus: PLANNED\n"
            f"Task Base: {base}\nContract Checkpoint: NOT SET\n"
            "Implementation Head: NOT SET\nDifficulty: Medium\nTest Level: T3\n"
            "Independent B Review: PENDING\nReal Telegram Visual Check: NOT RUN\n"
            "\n## Goal\nA clearer notification.\n\n## Task Report\nNot started.\n")
        (self.root / "TASK.md").write_text(plan)
        checkpoint = self.commit()
        if activate:
            self.edit("TASK.md", "# NO ACTIVE IMPLEMENTATION TASK", "# Task FIX-TG-01")
            self.edit("TASK.md", "Status: PLANNED", "Status: IN PROGRESS")
            self.edit("TASK.md", "Contract Checkpoint: NOT SET", "Contract Checkpoint: " + checkpoint)
        return base, checkpoint

    def test_standalone_plan_does_not_authorize_implementation(self):
        self.standalone_fixture(activate=False)
        self.assertIn("not implementation authorization", governance.validate(self.root))

    def test_standalone_activation_preserves_closed_slice(self):
        _, checkpoint = self.standalone_fixture()
        original = (self.root / "SLICE.md").read_text()
        self.assertIn("committed standalone repair", governance.validate(self.root))
        self.commit()
        self.assertIn("committed standalone repair", governance.validate(self.root))
        self.assertEqual(original, self.git("show", checkpoint + ":SLICE.md") + "\n")

    def test_standalone_activation_requires_a_committed_plan(self):
        base, checkpoint = self.standalone_fixture()
        self.edit("TASK.md", checkpoint, "NOT SET")
        with self.assertRaisesRegex(ValueError, "committed plan checkpoint"):
            governance.validate(self.root)
        self.edit("TASK.md", "Contract Checkpoint: NOT SET", "Contract Checkpoint: " + base)
        with self.assertRaises(ValueError):
            governance.validate(self.root)

    def test_standalone_rejects_invalid_or_unrelated_base(self):
        base, _ = self.standalone_fixture()
        self.git("switch", "-c", "unmerged")
        (self.root / "unmerged.txt").write_text("Unmerged fixture\n")
        unrelated = self.commit()
        self.git("switch", "main")
        # Switching restores the committed plan; activate it again.
        checkpoint = self.git("rev-parse", "HEAD")
        self.edit("TASK.md", "# NO ACTIVE IMPLEMENTATION TASK", "# Task FIX-TG-01")
        self.edit("TASK.md", "Status: PLANNED", "Status: IN PROGRESS")
        self.edit("TASK.md", "Contract Checkpoint: NOT SET", "Contract Checkpoint: " + checkpoint)
        for invalid in ("NOT SET", "0" * 40, unrelated):
            with self.subTest(base=invalid):
                self.edit("TASK.md", "Task Base: " + base, "Task Base: " + invalid)
                with self.assertRaises(ValueError):
                    governance.validate(self.root)
                self.edit("TASK.md", "Task Base: " + invalid, "Task Base: " + base)

    def test_standalone_base_cannot_follow_plan_checkpoint(self):
        base, _ = self.standalone_fixture()
        (self.root / "later.txt").write_text("Later fixture\n")
        later = self.commit()
        self.edit("TASK.md", "Task Base: " + base, "Task Base: " + later)
        with self.assertRaisesRegex(ValueError, "Git boundary"):
            governance.validate(self.root)

    def test_standalone_rejects_changed_scope_before_and_after_commit(self):
        self.standalone_fixture()
        self.edit("TASK.md", "A clearer notification.", "A new product feature.")
        with self.assertRaisesRegex(ValueError, "differs from its committed plan"):
            governance.validate(self.root)
        self.commit()
        with self.assertRaisesRegex(ValueError, "differs from its committed plan"):
            governance.validate(self.root)

    def test_standalone_reports_can_advance_without_changing_plan(self):
        _, checkpoint = self.standalone_fixture()
        self.edit("TASK.md", "Not started.", "Governance activated; product tests NOT RUN.")
        self.edit("TASK.md", "Implementation Head: NOT SET", "Implementation Head: " + checkpoint)
        self.edit("TASK.md", "Status: IN PROGRESS", "Status: READY FOR B REVIEW")
        self.assertIn("committed standalone repair", governance.validate(self.root))

    def test_standalone_cannot_hide_active_status(self):
        self.standalone_fixture()
        self.edit("TASK.md", "# Task FIX-TG-01", "# NO ACTIVE IMPLEMENTATION TASK")
        with self.assertRaisesRegex(ValueError, "no-active marker"):
            governance.validate(self.root)

    def test_standalone_rejects_mixed_parent_and_unknown_type(self):
        self.standalone_fixture()
        changes = (
            ("Parent Slice: NONE", "Parent Slice: 1"),
            ("Related Slice: 1", "Related Slice: 2"),
            ("Task ID: FIX-TG-01", "Task ID: 1.2"),
            ("Task Type: standalone-fix", "Task Type: new-feature"),
        )
        for before, after in changes:
            with self.subTest(value=after):
                self.edit("TASK.md", before, after)
                with self.assertRaises(ValueError):
                    governance.validate(self.root)
                self.edit("TASK.md", after, before)

    def test_standalone_requires_closed_related_slice(self):
        self.standalone_fixture()
        self.edit("SLICE.md", "Status: PASS / CLOSED", "Status: ACTIVE")
        self.edit("docs/roadmap.md", "| PASS / CLOSED |", "| ACTIVE |")
        self.commit()
        with self.assertRaisesRegex(ValueError, "closed related Slice"):
            governance.validate(self.root)

    def test_standalone_cannot_rewrite_historical_closure(self):
        self.standalone_fixture()
        self.edit("SLICE.md", "Decision: PASS", "Decision: PASS\nChanged historical evidence.")
        self.commit()
        with self.assertRaisesRegex(ValueError, "rewrite the closed Slice"):
            governance.validate(self.root)

    def test_standalone_review_requires_head_and_independent_pass(self):
        _, checkpoint = self.standalone_fixture()
        self.edit("TASK.md", "Status: IN PROGRESS", "Status: READY FOR B REVIEW")
        with self.assertRaisesRegex(ValueError, "implementation SHA"):
            governance.validate(self.root)
        self.edit("TASK.md", "Implementation Head: NOT SET", "Implementation Head: " + checkpoint)
        self.edit("TASK.md", "Status: READY FOR B REVIEW", "Status: PASS")
        with self.assertRaisesRegex(ValueError, "independent B PASS"):
            governance.validate(self.root)
        self.edit("TASK.md", "Independent B Review: PENDING", "Independent B Review: PASS")
        self.edit("TASK.md", "# Task FIX-TG-01", "# NO ACTIVE IMPLEMENTATION TASK")
        self.assertIn("committed standalone repair", governance.validate(self.root))

    def test_legacy_child_task_still_rejected_under_closed_slice(self):
        self.standalone_fixture()
        (self.root / "TASK.md").write_text("# Task 1.2\nParent Slice: 1\nStatus: PLANNED\n")
        with self.assertRaisesRegex(ValueError, "Closed Slice requires A PASS"):
            governance.validate(self.root)
