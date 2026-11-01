"""Regression: govulncheck JSON findings must still block publication.

Uses the installed govulncheck converter with synthetic scan results; no packages
are built and no real vulnerable dependency is added to the repository.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class GoScanGateTest(unittest.TestCase):
    def test_gate(self):
        scanner = shutil.which("govulncheck")
        self.assertIsNotNone(scanner, "Install the pinned govulncheck first")
        config = {"config": {"protocol_version": "v1.0.0", "scan_level": "symbol", "scan_mode": "source"}}
        vulnerable = [config, {"osv": {
            "id": "GO-TEST-0001", "summary": "Synthetic reachable vulnerability",
            "database_specific": {"url": "https://example.invalid/GO-TEST-0001"},
            "affected": [{"package": {"name": "example.com/fixture", "ecosystem": "Go"}}],
        }}, {"finding": {
            "osv": "GO-TEST-0001", "fixed_version": "v1.1.0",
            "trace": [{"module": "example.com/fixture", "version": "v1.0.0",
                       "package": "example.com/fixture", "function": "Vulnerable"}],
        }}]
        script = Path(__file__).with_name("scan-go.sh").resolve()
        with tempfile.TemporaryDirectory(prefix="maker-scan-gate-") as directory:
            root = Path(directory)
            stub = root / "govulncheck"
            stub.write_text("#!/usr/bin/env python3\n"
                            "import os,sys\n"
                            "if '-mode=convert' in sys.argv:\n"
                            " os.execv(os.environ['REAL_GOVULNCHECK'], [os.environ['REAL_GOVULNCHECK'], *sys.argv[1:]])\n"
                            "sys.stdout.write(os.environ['SCAN_FIXTURE'])\n"
                            "sys.exit(int(os.environ['SCAN_EXIT']))\n")
            stub.chmod(0o755)
            for name, messages, source_exit, expected in [
                ("clean", [config], 0, 0),
                ("reachable", vulnerable, 0, 3),
                ("scan-error", [config], 2, 2),
            ]:
                with self.subTest(name=name):
                    env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                               REAL_GOVULNCHECK=scanner,
                               SCAN_FIXTURE="\n".join(json.dumps(m) for m in messages),
                               SCAN_EXIT=str(source_exit))
                    result = subprocess.run(["bash", str(script), str(root / name)],
                                            env=env, capture_output=True, text=True, timeout=60)
                    self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                    self.assertTrue((root / f"{name}.json").exists())


if __name__ == "__main__":
    unittest.main()
