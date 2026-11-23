import QRCode from "qrcode";
import { useEffect, useState } from "react";

/** QR-код всегда на белом фоне — так его читают камеры и в тёмной теме. */
export function QrImage({ value, size = 220 }: { value: string; size?: number }) {
  const [src, setSrc] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setFailed(false);
    QRCode.toDataURL(value, { width: 280, margin: 2, errorCorrectionLevel: "L" })
      .then((url) => !cancelled && setSrc(url))
      .catch(() => !cancelled && setFailed(true));
    return () => {
      cancelled = true;
    };
  }, [value]);

  if (failed) return <p className="text-sm text-destructive">Не удалось построить QR-код (ссылка слишком длинная)</p>;
  return (
    <div
      className="grid shrink-0 place-items-center self-center justify-self-center overflow-hidden rounded-lg border bg-white"
      style={{ width: size, height: size }}
    >
      {src ? (
        <img src={src} alt="QR-код" width={size} height={size} />
      ) : (
        <span className="text-sm text-neutral-500">Генерация...</span>
      )}
    </div>
  );
}
