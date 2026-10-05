import plistlib
from pathlib import Path
import tempfile
import unittest
import xml.etree.ElementTree as ET

from network_config import ANDROID, INTERNET, configure_android, configure_ios, verify_android, verify_ios


class NetworkConfigTest(unittest.TestCase):
    def test_android_missing_permission_is_rejected_and_repaired_idempotently(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "AndroidManifest.xml"
            path.write_text('''<manifest xmlns:android="http://schemas.android.com/apk/res/android">
                <uses-permission android:name="existing.permission"/>
                <application android:label="S-UI Next" android:usesCleartextTraffic="true"/>
            </manifest>''')
            with self.assertRaisesRegex(ValueError, "INTERNET"):
                verify_android(ET.parse(path).getroot())
            configure_android(path)
            configure_android(path)
            root = ET.parse(path).getroot()
            verify_android(root)
            permissions = [node.get(ANDROID + "name") for node in root.findall("uses-permission")]
            self.assertEqual(permissions.count(INTERNET), 1)
            self.assertIn("existing.permission", permissions)
            self.assertEqual(root.find("application").get(ANDROID + "label"), "S-UI Next")

    def test_android_http_disabled_is_rejected(self):
        root = ET.fromstring(f'''<manifest xmlns:android="{ANDROID[1:-1]}">
            <uses-permission android:name="{INTERNET}"/><application/>
        </manifest>''')
        with self.assertRaisesRegex(ValueError, "HTTP"):
            verify_android(root)

    def test_ios_local_network_and_http_are_checked_in_built_plist(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "Info.plist"
            path.write_bytes(plistlib.dumps({"CFBundleIdentifier": "io.sui.sui_mobile"}))
            with self.assertRaisesRegex(ValueError, "local network"):
                verify_ios(path)
            configure_ios(path)
            configure_ios(path)
            verify_ios(path)
            data = plistlib.loads(path.read_bytes())
            self.assertEqual(data["CFBundleIdentifier"], "io.sui.sui_mobile")
            data["NSAppTransportSecurity"]["NSAllowsArbitraryLoads"] = False
            path.write_bytes(plistlib.dumps(data))
            with self.assertRaisesRegex(ValueError, "HTTP"):
                verify_ios(path)


if __name__ == "__main__":
    unittest.main()
