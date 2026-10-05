# Amostra do Zone. Arquivo original: api-py/app/sessions_repo.py
# O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

"""Opaque, rotating refresh credentials; plaintext credentials never reach SQL."""
from __future__ import annotations

from datetime import datetime, timedelta, timezone
import hashlib
import hmac
import re
import secrets
import uuid

from app.db import connection
from app.settings import jwt_secret

LIFETIME = timedelta(days=30)
RETRY_GRACE = timedelta(seconds=30)


class InvalidSessionError(ValueError):
    """Unknown, expired, revoked or replayed session."""


def _digest(token: str) -> str:
    return hashlib.sha256(token.encode("ascii")).hexdigest()


def _session_id(token: str) -> str:
    if not re.fullmatch(r"[0-9a-f-]{36}\.[A-Za-z0-9_-]{43,64}", token):
        raise InvalidSessionError
    try:
        return str(uuid.UUID(token.split(".", 1)[0]))
    except ValueError:
        raise InvalidSessionError from None


def _successor(token: str) -> str:
    # Deterministic only for a single credential: concurrent tabs/lost responses
    # can obtain the SAME successor during the short grace window. Hashes alone
    # cannot derive it without the server key and previous plaintext credential.
    secret = hmac.new(jwt_secret().encode(), ("zone-refresh:" + token).encode(), hashlib.sha256)
    return token.split(".", 1)[0] + "." + secret.hexdigest()


def create(user_id: str) -> tuple[str, datetime]:
    sid = str(uuid.uuid4())
    token = sid + "." + secrets.token_urlsafe(32)
    expiry = datetime.now(timezone.utc) + LIFETIME
    with connection() as conn:
        conn.execute("delete from auth_sessions where expires_at <= %s", (datetime.now(timezone.utc),))
        conn.execute("insert into auth_sessions (id, user_id, token_hash, expires_at) values (%s, %s, %s, %s)",
                     (sid, str(user_id), _digest(token), expiry))
    return token, expiry


def rotate(token: str, allowed: frozenset[str] | None = None) -> tuple[dict, str, datetime]:
    sid = _session_id(token)
    fingerprint = _digest(token)
    now = datetime.now(timezone.utc)
    result = None
    with connection() as conn:
        row = conn.execute("select s.user_id, s.token_hash, s.previous_hash, s.rotated_at, s.expires_at, "
                           "u.email, u.display_name from auth_sessions s join users u on u.id = s.user_id "
                           "where s.id = %s for update of s", (sid,)).fetchone()
        if row:
            user_id, current, previous, rotated, expiry, email, name = row
            user = {"id": user_id, "email": email, "display_name": name}
            if expiry <= now or (allowed is not None and str(user_id) not in allowed):
                conn.execute("delete from auth_sessions where id = %s", (sid,))
            elif hmac.compare_digest(current, fingerprint):
                fresh = _successor(token)
                conn.execute("update auth_sessions set previous_hash = token_hash, token_hash = %s, "
                             "rotated_at = %s where id = %s", (_digest(fresh), now, sid))
                result = (user, fresh, expiry)
            elif previous and hmac.compare_digest(previous, fingerprint):
                if now - rotated <= RETRY_GRACE:
                    fresh = _successor(token)
                    if hmac.compare_digest(current, _digest(fresh)):
                        result = (user, fresh, expiry)
                else:
                    # A consumed credential used after its retry window revokes
                    # the family. Commit deletion before raising the public error.
                    conn.execute("delete from auth_sessions where id = %s", (sid,))
            # A random token sharing the public UUID must NOT revoke a session.
    if result is None:
        raise InvalidSessionError
    return result


def revoke(token: str) -> None:
    try:
        sid = _session_id(token)
    except InvalidSessionError:
        return
    fingerprint = _digest(token)
    with connection() as conn:
        conn.execute("delete from auth_sessions where id = %s and (token_hash = %s or previous_hash = %s)",
                     (sid, fingerprint, fingerprint))
