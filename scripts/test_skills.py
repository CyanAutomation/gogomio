import tempfile
import unittest
from pathlib import Path

from check_skills import validate_skills


class ValidateSkillsTests(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.skills = self.root / ".github" / "skills"
        self.skill = self.skills / "camera-patterns"
        self.skill.mkdir(parents=True)
        (self.root / "docs").mkdir()
        (self.root / "docs" / "guide.md").write_text("# Guide\n")
        (self.root / "internal" / "camera").mkdir(parents=True)
        (self.root / "internal" / "camera" / "camera_test.go").write_text(
            "package camera\n"
            "import \"testing\"\n"
            "func TestCameraStart(t *testing.T) {}\n"
            "func TestCameraStop(t *testing.T) {}\n"
            "func BenchmarkCameraStart(b *testing.B) {}\n"
        )
        (self.skills / "README.md").write_text(
            "# Skills\n\n[Camera patterns](./camera-patterns/SKILL.md)\n"
        )
        (self.skill / "SKILL.md").write_text(
            "---\n"
            "name: camera-patterns\n"
            "description: Camera lifecycle guidance.\n"
            "---\n\n"
            "See [the guide](../../../docs/guide.md).\n\n"
            "```bash\n"
            "go test ./internal/camera -run 'TestCameraStart|TestCameraStop' -race\n"
            "go test ./internal/camera -run '^$' -bench '^BenchmarkCameraStart$'\n"
            "```\n"
        )

    def tearDown(self):
        self.tempdir.cleanup()

    def test_accepts_matching_metadata_links_and_existing_test_selectors(self):
        self.assertEqual([], validate_skills(self.root))

    def test_rejects_directory_name_mismatch_and_missing_catalog_entry(self):
        skill_path = self.skill / "SKILL.md"
        skill_path.write_text(skill_path.read_text().replace("name: camera-patterns", "name: camera"))
        (self.skills / "README.md").write_text("# Skills\n")

        errors = validate_skills(self.root)

        self.assertTrue(any("name must match directory" in error for error in errors))
        self.assertTrue(any("README catalog" in error for error in errors))

    def test_rejects_broken_markdown_links_and_nonexistent_test_selectors(self):
        skill_path = self.skill / "SKILL.md"
        skill_path.write_text(
            skill_path.read_text()
            .replace("../../../docs/guide.md", "../../../docs/missing.md")
            .replace("TestCameraStart|TestCameraStop", "TestMissing")
        )

        errors = validate_skills(self.root)

        self.assertTrue(any("broken Markdown link" in error for error in errors))
        self.assertTrue(any("matches no test" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
