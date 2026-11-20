"""Exercise the recovered Windows EXE on loopback with a temporary runtime."""

import http.cookiejar
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener

from stage_legacy_userdata import stage_legacy_userdata


ROOT = Path(__file__).resolve().parents[1]
EXE = ROOT / "dist/qa/HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe"


def smoke_once(runtime, password, bootstrap, expected_history=None, check_accounts=None):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    url = f"http://127.0.0.1:{port}"
    env = dict(os.environ, HSPO_ALLOW_INSECURE_RECOVERY_QA="I_ACKNOWLEDGE_INSECURE_QA_ONLY")
    env.pop("HSPO_RECOVERY_QA_ADMIN_PASSWORD", None)
    if bootstrap:
        env["HSPO_RECOVERY_QA_ADMIN_PASSWORD"] = password
    client = build_opener(ProxyHandler({}), HTTPCookieProcessor(http.cookiejar.CookieJar()))
    with subprocess.Popen(
        [str(EXE), "-root", str(runtime), "-http", str(port)],
        env=env,
        creationflags=subprocess.CREATE_NO_WINDOW,
    ) as process:
        try:
            for _ in range(50):
                if process.poll() is not None:
                    raise RuntimeError(f"Recovered Core exited with code {process.returncode}")
                try:
                    with client.open(url + "/health", timeout=1) as response:
                        assert json.load(response)["ok"] is True
                    break
                except URLError:
                    time.sleep(0.1)
            else:
                raise RuntimeError("Recovered Core did not become ready")

            request = Request(
                url + "/api/login",
                data=json.dumps({"username": "admin", "password": password}).encode(),
                headers={"Content-Type": "application/json"},
            )
            with client.open(request, timeout=2) as response:
                assert json.load(response)["user"]["role"] == "admin"
            with client.open(url + "/api/me", timeout=2) as response:
                assert json.load(response)["user"]["username"] == "admin"
            with client.open(url + "/static/tariff-warnings.js", timeout=2) as response:
                assert b"peerRate > rate" in response.read()
            if check_accounts is not None:
                check_accounts(url, client)
            if expected_history is not None:
                with client.open(url + "/api/search-history", timeout=2) as response:
                    recent = json.load(response)["recent"]
                assert len(recent) == 1
                assert all(recent[0].get(key) == value for key, value in expected_history.items())
        finally:
            if process.poll() is None:
                process.terminate()
                process.wait(timeout=5)


def api(url, client, path, expected_status, payload=None):
    body = None if payload is None else json.dumps(payload).encode()
    request = Request(url + path, body, {"Content-Type": "application/json"})
    try:
        response = client.open(request, timeout=3)
    except HTTPError as error:
        response = error
    with response:
        assert response.code == expected_status, (path, response.code, expected_status)
        return json.load(response)


def account_checks():
    username = "qa_staff"
    old_password = secrets.token_urlsafe(24)
    new_password = secrets.token_urlsafe(24)
    reset_password = secrets.token_urlsafe(24)
    users_path = "/api/admin/users"
    password_path = "/api/account/password"

    def login(url, client, password, status=200):
        return api(url, client, "/api/login", status, {
            "username": username, "password": password,
        })

    def browser():
        return build_opener(ProxyHandler({}), HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def before_restart(url, admin):
        user = browser()
        add = {"action": "add", "username": username, "target": username,
               "display_name": "Tài khoản thử", "role": "staff", "active": True}
        api(url, admin, users_path, 400, add)
        api(url, admin, users_path, 200, {**add, "password": old_password})
        api(url, admin, users_path, 409, {**add, "password": old_password})
        api(url, admin, users_path, 200, {
            **add, "username": "qa_other", "target": "qa_other", "password": old_password,
        })
        edit = {"action": "edit", "username": username}
        api(url, admin, users_path, 200, {**edit, "display_name": "Tên đã đổi"})
        profile = login(url, user, old_password)["user"]
        assert profile["display_name"] == "Tên đã đổi" and "password" not in profile
        api(url, user, users_path, 403, edit)
        api(url, user, password_path, 400, {
            "current_password": "wrong", "new_password": new_password,
        })
        stale = browser()
        login(url, stale, old_password)
        api(url, user, password_path, 200, {
            "current_password": old_password, "new_password": new_password,
        })
        api(url, user, "/api/me", 200)
        api(url, stale, "/api/me", 401)
        login(url, browser(), old_password, 401)
        login(url, browser(), new_password)
        api(url, admin, users_path, 409, {"action": "edit", "username": "admin", "active": False})
        api(url, admin, users_path, 200, {**edit, "active": False})
        api(url, user, "/api/me", 401)
        login(url, browser(), new_password, 401)
        rows = api(url, admin, "/api/users", 200)
        assert [(row["username"], row["active"]) for row in rows] == [
            ("admin", True), ("qa_other", True), (username, False),
        ]
        api(url, admin, users_path, 200, {**edit, "active": True})
        api(url, user, "/api/me", 401)
        login(url, user, new_password)
        api(url, admin, users_path, 200, {**edit, "password": reset_password})
        api(url, user, "/api/me", 401)
        login(url, browser(), new_password, 401)
        api(url, admin, users_path, 200, {"action": "delete", "username": "qa_other"})
        login(url, user, reset_password)
        api(url, user, "/api/search-history", 200, {"module": "tariff", "query": "01013010"})
        result = api(url, admin, users_path, 200, {"action": "delete", "username": username})
        assert result["disabled_instead_of_deleted"]
        api(url, admin, users_path, 200, {**edit, "active": True})
        api(url, admin, users_path, 200, {
            "action": "add", "username": "qa_locked", "password": old_password, "active": False,
        })

    def after_restart(url, admin):
        user = browser()
        assert login(url, user, reset_password)["user"]["display_name"] == "Tên đã đổi"
        login(url, browser(), old_password, 401)
        rows = api(url, admin, "/api/users", 200)
        assert [(row["username"], row["active"]) for row in rows] == [
            ("admin", True), (username, True), ("qa_locked", False),
        ]
        assert api(url, admin, "/api/search-history", 200)["recent"][0]["username"] == username
        other = browser()
        api(url, admin, users_path, 200, {
            "action": "add", "username": "qa_other", "password": old_password,
            "display_name": "Tên đã đổi",
        })
        api(url, other, "/api/login", 200, {"username": "qa_other", "password": old_password})
        history = "/api/search-history"
        api(url, other, history, 200, {"module": "tariff", "query": "01013090"})
        api(url, other, history, 200, {"module": "msds_file", "query": "qa.pdf"})
        for client, counts, search_counts, queries in [
            (user, {username: 1}, {username: 1}, ["01013010"]),
            (other, {"qa_other": 2}, {"qa_other": 1}, ["qa.pdf", "01013090"]),
            (admin, {username: 1, "qa_other": 2}, {username: 1, "qa_other": 1},
             ["qa.pdf", "01013090", "01013010"]),
        ]:
            result = api(url, client, history, 200)
            assert result["total"] == 3 and result["by_module"] == {"tariff": 2, "msds_file": 1}
            assert result["by_user"] == counts and result["by_user_search"] == search_counts
            assert [row["query"] for row in result["recent"]] == queries
            limited = api(url, client, history + "?limit=1", 200)
            assert [row["query"] for row in limited["recent"]] == queries[:1]
            assert limited["by_user_search"] == search_counts
        for endpoint in ("/api/search-history/export.csv", "/api/history.xlsx"):
            api(url, user, endpoint, 403)
            api(url, admin, endpoint, 501)

    return before_restart, after_restart


if __name__ == "__main__":
    with tempfile.TemporaryDirectory(prefix="hspo-recovered-qa-") as directory:
        runtime = Path(directory)
        shutil.copytree(ROOT / "apps/release-v198/web/static", runtime / "static")
        password = secrets.token_urlsafe(24)
        before_restart, after_restart = account_checks()
        smoke_once(runtime, password, bootstrap=True, check_accounts=before_restart)
        smoke_once(runtime, password, bootstrap=False, check_accounts=after_restart)
    with tempfile.TemporaryDirectory(prefix="hspo-legacy-qa-") as directory:
        source = Path(directory) / "userdata"
        source.mkdir()
        runtime = Path(directory) / "isolated"
        password = secrets.token_urlsafe(24)
        account = {
            "username": "admin", "role": "admin", "active": True,
            "password": hashlib.sha256(password.encode()).hexdigest(),
            "displayName": "QA fixture",
        }
        history = {
            "id": "qa-search", "ip": "127.0.0.1", "username": "admin",
            "module": "tariff", "query": "01013010", "result_count": 1,
        }
        fixture = {"users.json": json.dumps([account]), "search_history.jsonl": json.dumps(history)}
        for name, content in fixture.items():
            (source / name).write_text(content, encoding="utf8")
        assert stage_legacy_userdata(source, runtime) == (1, 1)
        shutil.copytree(ROOT / "apps/release-v198/web/static", runtime / "static")
        smoke_once(runtime, password, bootstrap=False, expected_history=history)
        assert all((source / name).read_text(encoding="utf8") == text for name, text in fixture.items())
    print("Windows recovered Core: startup, login, session, static file and restart passed.")
    print("QA accounts: create, rename, password changes, locks, sorting and restart passed.")
    print("QA history: own records and counts, global totals, limits and export permissions passed.")
    print("Synthetic legacy accounts and history staged and read through the Windows EXE.")
