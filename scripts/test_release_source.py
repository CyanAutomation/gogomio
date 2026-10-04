import subprocess
import tempfile
import unittest
from pathlib import Path

from validate_release_source import is_ancestor


class ValidateReleaseSourceTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo = Path(self.temp_dir.name)
        subprocess.run(["git", "init", "-q", "-b", "main"], cwd=self.repo, check=True)
        subprocess.run(["git", "config", "user.email", "test@example.test"], cwd=self.repo, check=True)
        subprocess.run(["git", "config", "user.name", "Workflow Test"], cwd=self.repo, check=True)
        (self.repo / "tracked.txt").write_text("main\n")
        subprocess.run(["git", "add", "tracked.txt"], cwd=self.repo, check=True)
        subprocess.run(["git", "commit", "-q", "-m", "main base"], cwd=self.repo, check=True)
        self.main_commit = self.git("rev-parse", "HEAD")

    def tearDown(self):
        self.temp_dir.cleanup()

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.repo, text=True).strip()

    def test_accepts_commit_reachable_from_main(self):
        self.assertTrue(is_ancestor(str(self.repo), self.main_commit, "main"))

    def test_rejects_commit_not_reachable_from_main(self):
        self.git("switch", "-q", "--orphan", "untrusted")
        (self.repo / "untrusted.txt").write_text("untrusted\n")
        self.git("add", "untrusted.txt")
        self.git("commit", "-q", "-m", "untrusted commit")
        untrusted_commit = self.git("rev-parse", "HEAD")
        self.assertFalse(is_ancestor(str(self.repo), untrusted_commit, "main"))


if __name__ == "__main__":
    unittest.main()
