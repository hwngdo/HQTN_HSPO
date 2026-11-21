"""Install the Windows QA package on a disposable path and exercise data updates."""

import argparse
import base64
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
from contextlib import contextmanager
from urllib.error import HTTPError, URLError
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener
import winreg
import zipfile


UNINSTALL_KEY = r"Software\Microsoft\Windows\CurrentVersion\Uninstall\{BDB1CA22-DF58-4474-929C-6D5938D32A9C}_is1"
UPDATE_ROOT = "/__hspo_admin/"
PROBE = "static/hspo_qa_update_probe.json"
REVISION = "QA-demo-1"


def fixture_package(path):
    content = b'{"qa_fixture": true, "version": 1}\n'
    manifest = {
        "format": "HQTN_HSPO_DATA_V1", "pack_id": "QA_DEMO", "min_app_version": "1.9.8",
        "data_version": REVISION, "rule_version": "2026.10.29.base",
        "description": "Gói thử QA, chỉ thêm tệp đánh dấu để kiểm tra cập nhật và khôi phục.",
        "legal_reviewed": True, "legal_review_date": "2026-11-21",
        "legal_review_evidence": "Đã kiểm tra gói thử chỉ chứa tệp đánh dấu và giữ nguyên dữ liệu nghiệp vụ.",
        "files": [{"path": PROBE, "size": len(content), "sha256": hashlib.sha256(content).hexdigest()}],
    }
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("manifest.json", json.dumps(manifest))
        archive.writestr(PROBE, content)


def run_setup(setup, target, env, password=None):
    child_env = env.copy()
    if password is not None:
        child_env["HSPO_RECOVERY_QA_ADMIN_PASSWORD"] = password
    options = ["/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-"]
    subprocess.run([str(setup), *options, f"/DIR={target}", f"/LOG={target}.setup.log"],
                   env=child_env, check=True, timeout=180, creationflags=subprocess.CREATE_NO_WINDOW)


def request(client, url, path, expect=200, *, method="GET", data=None, headers=None):
    item = Request(url + path, data=data, headers=headers or {}, method=method)
    try:
        response = client.open(item, timeout=4)
    except HTTPError as exc:
        response = exc
    with response:
        assert response.status == expect, (path, response.status, response.read(2048))
        return response.read()


def client_for(url, password):
    client = build_opener(ProxyHandler({}), HTTPCookieProcessor(http.cookiejar.CookieJar()))
    denied = json.loads(request(client, url, UPDATE_ROOT + "status", 403))
    assert not denied["ok"]
    payload = json.dumps({"username": "admin", "password": password}).encode()
    login = json.loads(request(client, url, "/api/login", method="POST", data=payload,
                               headers={"Content-Type": "application/json"}))
    assert login["user"]["role"] == "admin"
    return client


def status(client, url):
    return json.loads(request(client, url, UPDATE_ROOT + "status"))


def update_request(client, url, endpoint, csrf, *, payload=None, archive=None):
    headers = {"X-HSPO-CSRF": csrf}
    if archive is None:
        headers["Content-Type"] = "application/json"
        body = json.dumps(payload).encode()
    else:
        boundary = secrets.token_hex(24)
        headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
        body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="DATA_UPDATE.zip"\r\n'
                "Content-Type: application/zip\r\n\r\n").encode()
        body += archive.read_bytes() + f"\r\n--{boundary}--\r\n".encode()
    return json.loads(request(client, url, UPDATE_ROOT + endpoint, method="POST", data=body, headers=headers))


@contextmanager
def run_installed_tray(target, env):
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    url = f"http://127.0.0.1:{port}"
    args = [str(target / "HQTN_HSPO_Tray_QA.exe"), "-qa", "-background", f"-qa-port={port}"]
    with subprocess.Popen(args, cwd=target, env=env, creationflags=subprocess.CREATE_NO_WINDOW) as tray:
        try:
            client = build_opener(ProxyHandler({}))
            for _ in range(100):
                if tray.poll() is not None:
                    raise RuntimeError(f"Installed Tray exited with code {tray.returncode}")
                try:
                    with client.open(url + "/health", timeout=1) as response:
                        assert response.headers["X-HSPO-QA-PID"]
                        assert json.load(response)["ok"]
                    break
                except URLError:
                    time.sleep(0.1)
            else:
                raise RuntimeError("Installed Tray did not start its Core")
            yield url
        finally:
            if tray.poll() is None:
                tray.terminate()
                tray.wait(timeout=5)
            else:
                # A program upgrade replaces the original Tray process. This
                # cleanup only selects executables in this disposable install.
                cleanup_env = dict(os.environ, HSPO_SMOKE_INSTALL=str(target))
                cleanup = r"""
                $base = $env:HSPO_SMOKE_INSTALL
                $paths = @((Join-Path $base 'HQTN_HSPO_Tray_QA.exe'),
                           (Join-Path $base 'program_updates/HQTN_HSPO_Updater_QA.exe'))
                Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -in $paths } |
                  ForEach-Object { Stop-Process -Id $_.Id; [void]$_.WaitForExit(5000) }
                """
                subprocess.run(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", cleanup],
                               env=cleanup_env, check=True, timeout=20, creationflags=subprocess.CREATE_NO_WINDOW)
            for _ in range(50):
                with socket.socket() as sock:
                    if sock.connect_ex(("127.0.0.1", port)) != 0:
                        break
                time.sleep(0.1)
            else:
                raise RuntimeError("Installed Core did not exit with the test Tray")


def signed_version(manifest):
    signed = json.loads(manifest.read_bytes())
    return json.loads(base64.b64decode(signed["payload"]))["version"]


def verify_program_restart(target, url, setup, manifest, accounts, data_version):
    version = signed_version(manifest)
    parent = target / "program_updates"
    stage = parent / "fixture-download"
    stage.mkdir(parents=True)
    shutil.copy2(setup, stage / "HQTN_HSPO_QA_Setup_Win64.exe")
    shutil.copy2(manifest, stage / "HQTN_HSPO_QA_Release.json")
    stage.rename(parent / "pending")
    client = build_opener(ProxyHandler({}))
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            with client.open(url + "/health", timeout=1) as response:
                if (response.headers.get("X-HSPO-App-Release") == version
                        and not any(parent.glob("install-*/release/HQTN_HSPO_QA_Setup_Win64.exe"))):
                    break
        except (URLError, TimeoutError, ConnectionError):
            pass
        time.sleep(0.5)
    else:
        for log in (target / "program_updates").glob("install-*/setup.log"):
            print("\n".join(log.read_text(encoding="utf-8-sig", errors="replace").splitlines()[-30:]))
        raise RuntimeError("the installed update did not restart Tray/Core")
    for file, expected in accounts, data_version:
        assert file.read_bytes() == expected


def verify(setup, temp, demo, program_manifest=None):
    env = dict(os.environ)
    for key in ("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "HSPO_GITHUB_REPOSITORY", "HSPO_GITHUB_READ_TOKEN"):
        env.pop(key, None)
    password = secrets.token_urlsafe(25)
    target = temp / "Install HQTN QA"
    for view in (winreg.KEY_WOW64_64KEY, winreg.KEY_WOW64_32KEY):
        try:
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, UNINSTALL_KEY, 0, winreg.KEY_READ | view):
                raise RuntimeError("An installed HQTN_HSPO QA already exists for this user")
        except FileNotFoundError:
            pass
    try:
        run_setup(setup, target, env, password)
        accounts = target / "data/users.json"
        baseline = target / "static/hspo_data_version.json"
        account_bytes = accounts.read_bytes()
        baseline_bytes = baseline.read_bytes()
        assert len(json.loads(account_bytes)) == 1
        assert not (target / "userdata").exists()
        if program_manifest:
            version = signed_version(program_manifest)
            assert (target / "APP_RELEASE.txt").read_text().strip() == version
            assert (target / "static/VERSION.txt").read_text().strip() == version
            assert (target / "VERSION.txt").read_text().strip() == "1.9.8"
            # This exercise uses the updater build with an older version
            # marker, then upgrades it with the real signed installer.
            (target / "APP_RELEASE.txt").write_text("1.9.8\n", encoding="ascii")
        for asset in ("tools/tesseract/tesseract.exe", "static/tariff_2026.json.gz"):
            assert (target / asset).is_file()
        with run_installed_tray(target, env) as url:
            page = request(build_opener(ProxyHandler({})), url, "/").decode()
            assert "Kiểm tra gói ZIP offline" in page and "Kiểm tra GitHub online" in page
            client = client_for(url, password)
            before = status(client, url)
            assert before["github_enabled"] is False and not before["pending_restart"]
            if program_manifest:
                program = json.loads(request(client, url, UPDATE_ROOT + "program-status"))
                assert program["managed"] and program["current"] == "1.9.8"
                request(client, url, UPDATE_ROOT + "program-check", 403, method="POST")
                assert "Nội dung cập nhật" in page and "Cập nhật chương trình" in page
            denied = request(client, url, UPDATE_ROOT + "preview", 403, method="POST")
            assert not json.loads(denied)["ok"]
            preview = update_request(client, url, "preview", before["csrf"], archive=demo)
            assert preview["can_apply"] and preview["files"][0]["path"] == PROBE
            assert not (target / PROBE).exists()
            applied = update_request(client, url, "apply", before["csrf"], payload={
                "id": preview["id"], "confirm": "AP_DUNG",
            })
            assert (target / "data_updates/backups" / applied["backup"]).is_file()
            assert json.loads(request(client, url, "/" + PROBE))["qa_fixture"]
            after = status(client, url)
            assert after["data"]["data_version"] == REVISION and after["pending_restart"]
            if program_manifest:
                verify_program_restart(target, url, setup, program_manifest,
                                       (accounts, account_bytes), (baseline, baseline.read_bytes()))

        run_setup(setup, target, env)
        assert accounts.read_bytes() == account_bytes and (target / PROBE).is_file()
        assert json.loads(baseline.read_bytes())["data_version"] == REVISION
        github_configured = dict(env, HSPO_GITHUB_REPOSITORY="fixture/hspo", HSPO_GITHUB_READ_TOKEN="fixture-read-token")
        with run_installed_tray(target, github_configured) as url:
            client = client_for(url, password)
            after = status(client, url)
            assert after["data"]["data_version"] == REVISION
            assert not after["pending_restart"] and after["github_enabled"] is True
            update_request(client, url, "rollback", after["csrf"], payload={
                "file": applied["backup"], "confirm": "KHOI_PHUC",
            })
            assert not (target / PROBE).exists() and baseline.read_bytes() == baseline_bytes
            assert status(client, url)["pending_restart"]
    finally:
        uninstaller = target / "unins000.exe"
        if uninstaller.is_file():
            subprocess.run([str(uninstaller), "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"],
                           check=True, timeout=90, creationflags=subprocess.CREATE_NO_WINDOW)
    assert accounts.read_bytes() == account_bytes and baseline.read_bytes() == baseline_bytes
    assert not (target / "HQTN_HSPO_Core_QA.exe").exists()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("setup", type=Path)
    parser.add_argument("--demo-copy", type=Path)
    parser.add_argument("--program-manifest", type=Path)
    arguments = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hspo-installer-test-") as directory:
        temp = Path(directory)
        demo = temp / "DATA_UPDATE.zip"
        fixture_package(demo)
        try:
            verify(arguments.setup.resolve(strict=True), temp, demo, arguments.program_manifest)
        except Exception:
            for log in temp.glob("*.setup.log"):
                print("\n".join(log.read_text(encoding="utf-8-sig", errors="replace").splitlines()[-25:]))
            raise
        if arguments.demo_copy:
            shutil.copy2(demo, arguments.demo_copy)
    print("Installer, Tray, Admin bootstrap, offline preview/apply, reinstall, rollback, and uninstall verified.")
    if arguments.program_manifest:
        print("Signed program installer handed off from Tray, restarted, and retained the applied data.")
