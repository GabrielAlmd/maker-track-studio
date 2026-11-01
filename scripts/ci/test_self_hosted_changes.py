"""PR path selection must retain coverage for sources and CI configuration."""
import subprocess
from pathlib import Path
import unittest
import tempfile


class SelfHostedChangesTest(unittest.TestCase):
    def test_pr_changes(self):
        script = Path(__file__).with_name("self_hosted_changes.py")
        for paths, expected in [
            ([], False),
            ([b"README.md", b"docs/SELF_HOSTED.md"], False),
            ([b"apps/desktop/scripts/build.mjs"], False),
            ([b"apps/api/go.sum"], True),
            ([b"apps/web/package-lock.json"], True),
            ([b"deployments/self-hosted/Dockerfile"], True),
            ([b".github/workflows/security.yml"], True),
            ([b"scripts/ci/self_hosted_changes.py"], True),
            ([b".dockerignore"], True),
            # NUL separation must preserve filenames containing newlines/bytes.
            ([b"docs/note\napps/api/fake.md", b"docs/\xff.md"], False),
            ([b"docs/old.md", b"apps/api/renamed.go"], True),
        ]:
            with self.subTest(paths=paths):
                result = subprocess.run(["python3", str(script)], input=b"\0".join(paths),
                                        capture_output=True, check=True)
                value = b"true" if expected else b"false"
                self.assertEqual(result.stdout, b"self-hosted=" + value + b"\n")


    def test_depth_two_merge_includes_earlier_pr_commits(self):
        with tempfile.TemporaryDirectory(prefix="maker-pr-paths-") as directory:
            root = Path(directory)
            source = root / "source"
            source.mkdir()

            def git(*args):
                return subprocess.run(["git", "-C", str(source),
                                       "-c", "user.name=CI", "-c", "user.email=ci@example.invalid",
                                       *args], check=True, capture_output=True)

            git("init", "-b", "main")
            (source / "README.md").write_text("base")
            git("add", ".")
            git("commit", "-m", "base")
            git("checkout", "-b", "feature")
            (source / "apps/api").mkdir(parents=True)
            (source / "apps/api/fixture.go").write_text("package fixture")
            git("add", ".")
            git("commit", "-m", "API change in an earlier PR commit")
            (source / "README.md").write_text("docs follow-up")
            git("add", ".")
            git("commit", "-m", "latest PR commit only changes docs")
            git("checkout", "main")
            git("merge", "--no-ff", "feature", "-m", "synthetic PR merge")
            checkout = root / "checkout"
            subprocess.run(["git", "clone", "--depth=2", source.as_uri(), str(checkout)],
                           check=True, capture_output=True)
            paths = subprocess.run(["git", "-C", str(checkout), "diff", "--name-only",
                                    "--no-renames", "-z", "HEAD^1", "HEAD"],
                                   check=True, capture_output=True).stdout
            result = subprocess.run(["python3", str(Path(__file__).with_name("self_hosted_changes.py"))],
                                    input=paths, check=True, capture_output=True)
            self.assertEqual(result.stdout, b"self-hosted=true\n")


if __name__ == "__main__":
    unittest.main()
