#!/usr/bin/env python3
"""Configure host networking and verify the settings in release artifacts."""

import argparse
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import xml.etree.ElementTree as ET


ANDROID = "{http://schemas.android.com/apk/res/android}"
INTERNET = "android.permission.INTERNET"


def configure_android(path):
    ET.register_namespace("android", ANDROID[1:-1])
    tree = ET.parse(path)
    root = tree.getroot()
    if not any(node.get(ANDROID + "name") == INTERNET for node in root.findall("uses-permission")):
        root.insert(0, ET.Element("uses-permission", {ANDROID + "name": INTERNET}))
    application = root.find("application")
    if application is None:
        raise ValueError("Android manifest has no application")
    application.set(ANDROID + "usesCleartextTraffic", "true")
    ET.indent(tree, space="    ")
    tree.write(path, encoding="utf-8", xml_declaration=True)


def verify_android(root):
    permissions = {node.get(ANDROID + "name") for node in root.findall("uses-permission")}
    if INTERNET not in permissions:
        raise ValueError("Release APK is missing android.permission.INTERNET")
    application = root.find("application")
    if application is None or application.get(ANDROID + "usesCleartextTraffic") != "true":
        raise ValueError("Release APK cannot connect to user-configured HTTP panels")


def configure_ios(path):
    with path.open("rb") as source:
        data = plistlib.load(source)
    data.setdefault("NSAppTransportSecurity", {})["NSAllowsArbitraryLoads"] = True
    data["NSLocalNetworkUsageDescription"] = (
        "S-UI Next needs local network access to connect to the panel address you enter."
    )
    with path.open("wb") as destination:
        plistlib.dump(data, destination)


def verify_ios(path):
    with path.open("rb") as source:
        data = plistlib.load(source)
    if not data.get("NSLocalNetworkUsageDescription", "").strip():
        raise ValueError("iPhone app is missing its local network usage description")
    if data.get("NSAppTransportSecurity", {}).get("NSAllowsArbitraryLoads") is not True:
        raise ValueError("iPhone app cannot connect to user-configured HTTP panels")


def verify_apk(path):
    analyzer = shutil.which("apkanalyzer")
    if not analyzer:
        for variable in ("ANDROID_HOME", "ANDROID_SDK_ROOT"):
            sdk = os.environ.get(variable)
            if sdk:
                candidate = Path(sdk) / "cmdline-tools/latest/bin/apkanalyzer"
                if candidate.is_file():
                    analyzer = str(candidate)
                    break
    if not analyzer:
        raise RuntimeError("apkanalyzer is required to inspect the packaged Android manifest")
    manifest = subprocess.check_output([analyzer, "manifest", "print", str(path)], text=True)
    verify_android(ET.fromstring(manifest))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("configure-android", "configure-ios", "verify-apk", "verify-ios"))
    parser.add_argument("path", type=Path)
    args = parser.parse_args()
    actions = {
        "configure-android": configure_android,
        "configure-ios": configure_ios,
        "verify-apk": verify_apk,
        "verify-ios": verify_ios,
    }
    actions[args.action](args.path)
    print(f"{args.action}: OK")
