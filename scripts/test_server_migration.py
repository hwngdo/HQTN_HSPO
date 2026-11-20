import tempfile
import unittest
from pathlib import Path

from server_migration import export, restore


def fixture(root):
    root.mkdir()
    (root / "VERSION.txt").write_text("1.9.8\n", encoding="utf-8")
    (root / "HQTN_HSPO_Core.exe").write_bytes(b"synthetic Core")
    (root / "HQTN_HSPO_TEST_Runtime_Server.exe").write_bytes(b"synthetic runtime server")
    (root / "policy_data").mkdir()
    (root / "policy_data/chemical_rules.json").write_text("[]", encoding="utf-8")
    (root / "static").mkdir()


class MigrationTests(unittest.TestCase):
    def test_same_version_roundtrip(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)
            first, second, bundle = (path / name for name in ("first", "second", "bundle"))
            fixture(first)
            fixture(second)
            users = b'[{"username":"fixture"}]'
            (first / "userdata/tls").mkdir(parents=True)
            (first / "userdata/users.json").write_bytes(users)
            (first / "userdata/tls/test-private.key").write_bytes(b"synthetic TLS key")
            self.assertEqual(export(first, bundle, False), 1)
            self.assertFalse(bundle.exists())
            self.assertEqual(export(first, bundle, True), 1)
            self.assertFalse((bundle / "userdata/tls").exists())
            self.assertEqual(restore(bundle, second, False), 1)
            self.assertFalse((second / "userdata").exists())
            self.assertEqual(restore(bundle, second, True), 1)
            self.assertEqual((second / "userdata/users.json").read_bytes(), users)
            self.assertEqual((first / "userdata/users.json").read_bytes(), users)
            with self.assertRaises(FileExistsError):
                restore(bundle, second, True)

    def test_rejects_different_data_and_modified_bundle(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)
            first, second, bundle = (path / name for name in ("first", "second", "bundle"))
            fixture(first)
            fixture(second)
            (first / "userdata").mkdir()
            (first / "userdata/users.json").write_text("[]", encoding="utf-8")
            export(first, bundle, True)
            target_data = second / "policy_data/chemical_rules.json"
            target_data.write_text('[{"fixture":1}]', encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "datasets differ"):
                restore(bundle, second, True)
            target_data.write_text("[]", encoding="utf-8")
            (bundle / "userdata/users.json").write_text('[{"tampered":true}]', encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                restore(bundle, second, True)
            self.assertFalse((second / "userdata").exists())


if __name__ == "__main__":
    unittest.main()
