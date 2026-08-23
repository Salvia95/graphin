import type { Decision, Workspace } from "@/api"

/** The URL that hands a decision's file to the reader's editor.
 *
 *  A link rather than an endpoint on purpose: the browser asks before handing
 *  off to an external application, so the person is in the loop. A loopback
 *  route that spawned a process would not have that, and the bind-address
 *  argument that lets this server skip authentication does not stretch to
 *  running programs.
 *
 *  Null whenever anything is unknown. A dead link is worse than no link — it
 *  teaches the reader that the button is a lie. */
export function editorHref(ws: Workspace | null, d: Decision): string | null {
  if (!ws?.editor_url || !d.file) return null
  let path = `${ws.root.replace(/[\\/]$/, "")}/${d.file}`.replace(/\\/g, "/")
  // vscode://file/C:/… — a Windows drive letter needs the leading slash the
  // POSIX root already has.
  if (/^[A-Za-z]:/.test(path)) path = `/${path}`
  return ws.editor_url
    .replace("{path}", encodeURI(path))
    .replace("{line}", String(d.line && d.line > 0 ? d.line : 1))
}
