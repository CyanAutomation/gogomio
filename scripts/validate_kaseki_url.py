#!/usr/bin/env python3
"""Validate and normalize the Kaseki controller URL against an exact host allowlist."""

import re
import sys
from urllib.parse import unquote, urlsplit


HOST_RE = re.compile(
    r"^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$",
    re.IGNORECASE,
)
MAX_PATH_DECODE_ROUNDS = 16


class KasekiURLValidationError(ValueError):
    pass


def _contains_parent_path_segment(path):
    normalized_path = path
    for _ in range(MAX_PATH_DECODE_ROUNDS):
        if ".." in normalized_path.replace("\\", "/").split("/"):
            return True
        decoded_path = unquote(normalized_path)
        if decoded_path == normalized_path:
            return False
        normalized_path = decoded_path

    if ".." in normalized_path.replace("\\", "/").split("/"):
        return True
    if unquote(normalized_path) != normalized_path:
        raise KasekiURLValidationError("KASEKI_BASE_URL path has too many encoding layers")
    return False


def _normalize_allowlist(allowed_hosts):
    hosts = set()
    for item in allowed_hosts.split(","):
        item = item.strip().lower().rstrip(".")
        if not item:
            continue
        host, separator, port_text = item.partition(":")
        if not HOST_RE.fullmatch(host):
            raise KasekiURLValidationError(
                "allowlist entries must be exact DNS names, optionally with a port"
            )
        if separator:
            if not port_text.isdecimal() or not 1 <= int(port_text) <= 65535:
                raise KasekiURLValidationError("allowlist port must be between 1 and 65535")
            item = "{}:{}".format(host, int(port_text))
        else:
            item = host
        hosts.add(item)
    if not hosts:
        raise KasekiURLValidationError(
            "KASEKI_ALLOWED_HOSTS must contain at least one exact host"
        )
    return hosts


def validate_base_url(value, allowed_hosts):
    if not value or any(character.isspace() or ord(character) < 32 for character in value):
        raise KasekiURLValidationError("KASEKI_BASE_URL must be set and contain no whitespace")
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError as error:
        raise KasekiURLValidationError("KASEKI_BASE_URL is malformed") from error

    if parsed.scheme.lower() != "https" or not parsed.hostname:
        raise KasekiURLValidationError("KASEKI_BASE_URL must be an HTTPS URL with a hostname")
    if parsed.username is not None or parsed.password is not None:
        raise KasekiURLValidationError("KASEKI_BASE_URL must not include credentials")
    if parsed.query or parsed.fragment:
        raise KasekiURLValidationError("KASEKI_BASE_URL must not include a query or fragment")

    hostname = parsed.hostname.lower().rstrip(".")
    if not HOST_RE.fullmatch(hostname):
        raise KasekiURLValidationError("KASEKI_BASE_URL must use a DNS hostname")
    authority = hostname + (":{}".format(port) if port is not None else "")
    if authority not in _normalize_allowlist(allowed_hosts):
        raise KasekiURLValidationError("KASEKI_BASE_URL hostname and port are not allowlisted")

    path = parsed.path.rstrip("/")
    if _contains_parent_path_segment(path):
        raise KasekiURLValidationError("KASEKI_BASE_URL path must not contain parent segments")
    return "https://{}{}".format(authority, path)


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 2:
        print("usage: validate_kaseki_url.py BASE_URL ALLOWED_HOSTS", file=sys.stderr)
        return 2
    try:
        print(validate_base_url(args[0], args[1]))
    except KasekiURLValidationError as error:
        print("Invalid Kaseki controller configuration: {}".format(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
