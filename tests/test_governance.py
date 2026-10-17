import json
import hashlib
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
            "STRM Generator Go.md": "# V4.3 test plan\n",
            ".go-version": "1.27.2\n",
            "go.mod": "module test-fixture\n\ngo 1.27.2\n",
            "README.md": "[architecture](docs/architecture.md)\n",
            "docs/architecture.md": "# test architecture\n",
            "docs/requirements.md": "# test requirements\n",
            "docs/roadmap.md": "| 1 | 安全单轮生成 | PLANNED |\n",
            "SLICE.md": "Slice ID: 1\nStatus: PLANNED\nBase SHA: NOT SET\nImplementation Head: NOT SET\n\n## Required Outcomes\nA safe result.\n\n## Closure Packet\nNOT STARTED\n",
            "TASK.md": "# NO ACTIVE IMPLEMENTATION TASK\n",
        }
        spec = {"plan": {"path": "STRM Generator Go.md", "sha256": hashlib.sha256(files["STRM Generator Go.md"].encode()).hexdigest()},
                "go": {"version": "1.27.2"}, "required_files": [*files, "dev/environment-spec.json"]}
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

    def test_changed_plan_is_rejected(self):
        self.edit("STRM Generator Go.md", "V4.3", "V9.9")
        with self.assertRaisesRegex(ValueError, "identity"):
            governance.validate(self.root, True)

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
        with self.assertRaisesRegex(ValueError, "Git boundary"):
            governance.validate(self.root)

    def test_committed_planning_is_valid_but_not_active(self):
        self.git("init", "-b", "main")
        self.commit()
        self.assertIn("no active implementation", governance.validate(self.root))

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
