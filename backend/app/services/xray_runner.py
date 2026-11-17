"""Исполнители команд для менеджера Xray: локально, по SSH и в памяти (dry-run/тесты)."""

from __future__ import annotations

import contextlib
import fcntl
import json
import os
import shlex
import subprocess
import threading
from collections.abc import Iterator
from pathlib import Path
from typing import Protocol


class XrayCommandError(RuntimeError):
    pass


class CommandRunner(Protocol):
    def run(self, args: list[str], *, timeout: int = 20) -> str: ...

    def read_text(self, path: str) -> str: ...

    def write_text(self, path: str, content: str) -> None: ...

    def replace(self, src: str, dst: str) -> None: ...

    def copy(self, src: str, dst: str) -> None: ...

    def list_dir(self, path: str) -> list[str]: ...

    def remove(self, path: str) -> None: ...

    def lock(self) -> contextlib.AbstractContextManager[None]: ...


class LocalRunner:
    """Панель стоит на том же сервере, что и Xray."""

    def __init__(self, lock_path: str) -> None:
        self._lock_path = lock_path
        self._thread_lock = threading.Lock()

    def run(self, args: list[str], *, timeout: int = 20) -> str:
        try:
            result = subprocess.run(args, capture_output=True, text=True, timeout=timeout, check=False)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise XrayCommandError(f"{shlex.join(args)}: {exc}") from exc
        if result.returncode != 0:
            raise XrayCommandError(
                f"{shlex.join(args)}: {(result.stderr or result.stdout).strip()[:500]}"
            )
        return result.stdout

    def read_text(self, path: str) -> str:
        return Path(path).read_text(encoding="utf-8")

    def write_text(self, path: str, content: str) -> None:
        Path(path).parent.mkdir(parents=True, exist_ok=True)
        Path(path).write_text(content, encoding="utf-8")

    def replace(self, src: str, dst: str) -> None:
        os.chmod(src, 0o644)  # Xray работает от nobody и должен читать конфиг
        os.replace(src, dst)

    def copy(self, src: str, dst: str) -> None:
        Path(dst).parent.mkdir(parents=True, exist_ok=True)
        Path(dst).write_bytes(Path(src).read_bytes())

    def list_dir(self, path: str) -> list[str]:
        return sorted(os.listdir(path)) if os.path.isdir(path) else []

    def remove(self, path: str) -> None:
        Path(path).unlink(missing_ok=True)

    @contextlib.contextmanager
    def lock(self) -> Iterator[None]:
        with self._thread_lock:
            Path(self._lock_path).parent.mkdir(parents=True, exist_ok=True)
            with open(self._lock_path, "a") as handle:
                fcntl.flock(handle, fcntl.LOCK_EX)
                try:
                    yield
                finally:
                    fcntl.flock(handle, fcntl.LOCK_UN)


class SshRunner:
    """Панель на отдельной машине, Xray — на входном сервере (доступ по SSH-ключу)."""

    def __init__(self, *, host: str, port: int, user: str, key_path: str, known_hosts: str | None) -> None:
        self._host, self._port, self._user, self._key_path = host, port, user, key_path
        self._known_hosts = known_hosts
        self._thread_lock = threading.Lock()

    def _connect(self):  # type: ignore[no-untyped-def]
        import paramiko

        client = paramiko.SSHClient()
        if self._known_hosts:
            client.load_host_keys(self._known_hosts)
            client.set_missing_host_key_policy(paramiko.RejectPolicy())
        else:
            client.set_missing_host_key_policy(paramiko.WarningPolicy())
        client.connect(
            hostname=self._host,
            port=self._port,
            username=self._user,
            key_filename=self._key_path,
            timeout=15,
        )
        return client

    def _shell(self, command: str, timeout: int = 20) -> str:
        with self._connect() as client:
            _, stdout, stderr = client.exec_command(command, timeout=timeout)
            code = stdout.channel.recv_exit_status()
            out = stdout.read().decode("utf-8")
            if code != 0:
                err = stderr.read().decode("utf-8").strip()
                raise XrayCommandError(f"{command}: {(err or out).strip()[:500]}")
            return out

    def run(self, args: list[str], *, timeout: int = 20) -> str:
        return self._shell(shlex.join(args), timeout)

    def read_text(self, path: str) -> str:
        return self._shell(f"cat {shlex.quote(path)}")

    def write_text(self, path: str, content: str) -> None:
        with self._connect() as client:
            sftp = client.open_sftp()
            with sftp.file(path, "wb") as remote:
                remote.write(content.encode("utf-8"))
            sftp.close()

    def replace(self, src: str, dst: str) -> None:
        self._shell(f"cat {shlex.quote(src)} > {shlex.quote(dst)} && rm -f {shlex.quote(src)}")

    def copy(self, src: str, dst: str) -> None:
        self._shell(f"mkdir -p {shlex.quote(os.path.dirname(dst))} && cp {shlex.quote(src)} {shlex.quote(dst)}")

    def list_dir(self, path: str) -> list[str]:
        out = self._shell(f"ls -1 {shlex.quote(path)} 2>/dev/null || true")
        return sorted(line for line in out.splitlines() if line)

    def remove(self, path: str) -> None:
        self._shell(f"rm -f {shlex.quote(path)}")

    @contextlib.contextmanager
    def lock(self) -> Iterator[None]:
        with self._thread_lock:
            yield


class MemoryRunner:
    """Файлы в памяти и имитация `xray` — для dry-run и тестов."""

    def __init__(self, files: dict[str, str] | None = None) -> None:
        self.files: dict[str, str] = dict(files or {})
        self.commands: list[list[str]] = []
        self.api_users: dict[str, set[str]] = {}
        self.stats: dict[str, int] = {}
        self.online: set[str] = set()
        self.fail_api = False
        self.fail_test = False
        self._thread_lock = threading.Lock()

    def run(self, args: list[str], *, timeout: int = 20) -> str:
        self.commands.append(list(args))
        if args[:2] == ["xray", "x25519"]:
            return "PrivateKey: dry-run-private\nPassword (PublicKey): dry-run-public-key\nHash32: x\n"
        if args[:3] == ["xray", "run", "-test"]:
            if not args[-1].endswith(".json"):
                raise XrayCommandError("Failed to get format of " + args[-1])
            if self.fail_test:
                raise XrayCommandError("config test failed")
            json.loads(self.files[args[-1]])
            return "Configuration OK."
        if args[:2] == ["xray", "api"]:
            return self._api(args[2], args[3:])
        return ""

    def _api(self, command: str, rest: list[str]) -> str:
        if self.fail_api:
            raise XrayCommandError("api unavailable")
        if command == "adu":
            payload = json.loads(self.files[rest[-1]])
            count = 0
            for inbound in payload["inbounds"]:
                for client in inbound["settings"]["clients"]:
                    self.api_users.setdefault(inbound["tag"], set()).add(client["email"])
                    count += 1
            return f"Added {count} user(s) in total.\n"
        if command == "rmu":
            tag = next(arg.split("=", 1)[1] for arg in rest if arg.startswith("-tag="))
            email = rest[-1]
            removed = int(email in self.api_users.get(tag, set()))
            self.api_users.get(tag, set()).discard(email)
            return f"Removed {removed} user(s) in total.\n"
        if command == "statsquery":
            stat = [{"name": name, "value": value} for name, value in self.stats.items()]
            if "-reset" in rest:
                self.stats = {}
            return json.dumps({"stat": stat} if stat else {})
        if command == "statsgetallonlineusers":
            users = [f"user>>>{email}>>>online" for email in sorted(self.online)]
            return json.dumps({"users": users} if users else {})
        return ""

    def read_text(self, path: str) -> str:
        if path not in self.files:
            raise XrayCommandError(f"{path}: not found")
        return self.files[path]

    def write_text(self, path: str, content: str) -> None:
        self.files[path] = content

    def replace(self, src: str, dst: str) -> None:
        self.files[dst] = self.files.pop(src)

    def copy(self, src: str, dst: str) -> None:
        self.files[dst] = self.files[src]

    def list_dir(self, path: str) -> list[str]:
        prefix = path.rstrip("/") + "/"
        return sorted(name[len(prefix):] for name in self.files if name.startswith(prefix))

    def remove(self, path: str) -> None:
        self.files.pop(path, None)

    @contextlib.contextmanager
    def lock(self) -> Iterator[None]:
        with self._thread_lock:
            yield
