/**
 * Короткий вид ключа для показа: «vless://9755086b…@kemsekov.ru:443». Полная ссылка длинная и в окно
 * не помещается, а по началу UUID и адресу ключ легко узнать. Нераспознанная ссылка возвращается как есть.
 */
export function keyPreview(uri: string) {
  const match = /^([a-z][a-z0-9+.-]*):\/\/([^@/?#]+)@([^/?#]+)/i.exec(uri.trim());
  if (!match) return uri;
  const [, scheme, id, host] = match;
  const head = id.length > 8 ? `${id.slice(0, 8)}…` : id;
  return `${scheme}://${head}@${host}`;
}
