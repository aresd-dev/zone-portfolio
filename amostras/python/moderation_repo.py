# Amostra do Zone. Arquivo original: api-py/app/moderation_repo.py
# O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

"""Small installation roles and persistent voice-room moderation."""
import os

from fastapi import HTTPException

from app.db import connection


def owner_email():
    # The principal administrator comes only from private configuration; an
    # empty value matches no account (other admins live in zone_admins).
    return os.getenv("ZONE_ADMIN_EMAIL", "").strip().lower() or None


def _admin(conn, user):
    return user["email"].lower() == owner_email() or conn.execute(
        "select 1 from zone_admins where user_id=%s", (str(user["id"]),)
    ).fetchone() is not None


def is_admin(user):
    if user["email"].lower() == owner_email():
        return True
    with connection() as conn:
        return _admin(conn, user)


def list_users(allowed):
    with connection() as conn:
        rows = conn.execute(
            "select u.id,u.display_name,u.email,a.user_id is not null "
            "from users u left join zone_admins a on a.user_id=u.id "
            "where (%s::uuid[] is null or u.id=any(%s::uuid[])) order by u.display_name",
            (list(allowed) if allowed is not None else None, list(allowed) if allowed is not None else None),
        ).fetchall()
    return [{"id": str(r[0]), "display_name": r[1],
             "is_admin": r[3] or r[2].lower() == owner_email(),
             "is_owner": r[2].lower() == owner_email()} for r in rows]


def set_admin(actor, target_id, enabled, allowed):
    with connection() as conn:
        conn.execute("select pg_advisory_xact_lock(1852143733, 2)")
        if not _admin(conn, actor):
            raise HTTPException(403, "apenas administradores podem alterar cargos")
        target = conn.execute("select email from users where id=%s", (str(target_id),)).fetchone()
        if target is None or (allowed is not None and str(target_id) not in allowed):
            raise HTTPException(404, "conta não encontrada")
        if target[0].lower() == owner_email():
            raise HTTPException(403, "o administrador principal não pode ser removido")
        if enabled:
            conn.execute("insert into zone_admins(user_id,granted_by) values(%s,%s) "
                         "on conflict(user_id) do nothing", (str(target_id), str(actor["id"])))
        else:
            conn.execute("delete from zone_admins where user_id=%s", (str(target_id),))


def _room(conn, actor, server_id, channel_id):
    row = conn.execute(
        "select c.id from channels c where c.id=%s and c.server_id=%s and c.kind='voice' "
        "and exists(select 1 from server_members m where m.server_id=c.server_id and m.user_id=%s)",
        (str(channel_id), str(server_id), str(actor["id"])),
    ).fetchone()
    if row is None:
        raise HTTPException(404, "canal de áudio não encontrado")


def _snapshot(conn, actor, channel_id):
    rows = conn.execute("select user_id,muted,volume,revision from voice_controls where channel_id=%s",
                        (str(channel_id),)).fetchall()
    return {"can_manage": _admin(conn, actor), "version": max((r[3] for r in rows), default=0),
            "items": [{"user_id": str(r[0]), "muted": r[1], "volume": r[2]} for r in rows]}


def snapshot(actor, server_id, channel_id):
    with connection() as conn:
        _room(conn, actor, server_id, channel_id)
        return _snapshot(conn, actor, channel_id)


def authorize_disconnect(actor, server_id, channel_id, target_id):
    """Administrators may remove a member from the call; the principal
    administrator cannot be removed by someone else."""
    with connection() as conn:
        _room(conn, actor, server_id, channel_id)
        if not _admin(conn, actor):
            raise HTTPException(403, "apenas administradores podem remover alguém da chamada")
        target = conn.execute(
            "select u.email from server_members m join users u on u.id=m.user_id "
            "where m.server_id=%s and m.user_id=%s", (str(server_id), str(target_id))).fetchone()
        if target is None:
            raise HTTPException(404, "participante não encontrado")
        if target[0].lower() == owner_email():
            raise HTTPException(403, "o administrador principal não pode ser removido da chamada")


def update(actor, server_id, channel_id, target_id, changes):
    with connection() as conn:
        _room(conn, actor, server_id, channel_id)
        # Serialize edits and revocations, rechecking the canonical role.
        conn.execute("select pg_advisory_xact_lock(1852143733, 2)")
        if not _admin(conn, actor):
            raise HTTPException(403, "apenas administradores podem moderar a chamada")
        if not conn.execute("select 1 from server_members where server_id=%s and user_id=%s",
                            (str(server_id), str(target_id))).fetchone():
            raise HTTPException(404, "participante não encontrado")
        conn.execute("insert into voice_controls(channel_id,user_id) values(%s,%s) "
                     "on conflict do nothing", (str(channel_id), str(target_id)))
        conn.execute("update voice_controls set muted=coalesce(%s,muted),volume=coalesce(%s,volume), "
                     "revision=nextval('voice_controls_revision') where channel_id=%s and user_id=%s",
                     (changes.get("muted"), changes.get("volume"), str(channel_id), str(target_id)))
        return _snapshot(conn, actor, channel_id)
