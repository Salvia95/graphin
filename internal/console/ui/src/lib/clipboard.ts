/** Copy text, without assuming the async clipboard API is there.
 *
 *  http://127.0.0.1 is a secure context, so navigator.clipboard normally works;
 *  the fallback exists because a console served from an --addr the user chose,
 *  or an older browser, would otherwise fail silently on the one affordance the
 *  card offers.
 */
export async function copy(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // fall through
  }
  try {
    const el = document.createElement("textarea")
    el.value = text
    el.setAttribute("readonly", "")
    el.style.position = "fixed"
    el.style.opacity = "0"
    document.body.appendChild(el)
    el.select()
    const ok = document.execCommand("copy")
    document.body.removeChild(el)
    return ok
  } catch {
    return false
  }
}
