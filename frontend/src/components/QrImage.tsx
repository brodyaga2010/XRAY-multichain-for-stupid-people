import QRCode from "qrcode";
import { useEffect, useState } from "react";

export function QrImage({ value }: { value: string }) {
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

  if (failed) return <p className="error">Не удалось построить QR-код (ссылка слишком длинная)</p>;
  if (!src) return <div className="qr-box muted">Генерация...</div>;
  return <img className="qr-box" src={src} alt="QR-код" width={280} height={280} />;
}
