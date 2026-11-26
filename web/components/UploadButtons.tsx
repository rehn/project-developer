"use client";
import { useEffect, useRef, useState } from "react";
import type { Project } from "@/lib/api";

/** Upload files or a folder to `.uploaded/` in the project (created if missing). */
export default function UploadButtons({ project }: { project: Project }) {
  const filePicker = useRef<HTMLInputElement>(null);
  const folderPicker = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [upMsg, setUpMsg] = useState<{ ok: boolean; text: string } | null>(null);
  useEffect(() => { setUpMsg(null); }, [project.id]);

  // Folder structure travels in the form field name: multipart filenames lose their directories server-side.
  async function upload(files: File[]) {
    if (!files.length) return;
    setUploading(true); setUpMsg(null);
    try {
      const fd = new FormData();
      for (const f of files) fd.append(encodeURIComponent(f.webkitRelativePath || f.name), f);
      const res = await fetch(`/api/projects/${project.id}/upload`, { method: "POST", body: fd });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error ?? `Upload failed (${res.status})`);
      setUpMsg({ ok: true, text: `Uploaded ${data.count} file${data.count === 1 ? "" : "s"} to ${data.dir}/` });
    } catch (e: any) { setUpMsg({ ok: false, text: e.message }); }
    finally { setUploading(false); }
  }

  return (
    <>
      <button onClick={() => filePicker.current?.click()} disabled={uploading} title="Upload files to .uploaded/ in the project">Upload files…</button>
      <button onClick={() => folderPicker.current?.click()} disabled={uploading} title="Upload a folder to .uploaded/ in the project">Upload folder…</button>
      {uploading && <span className="muted small">Uploading…</span>}
      {upMsg && <span className={`${upMsg.ok ? "ok" : "error"} small`} style={{ margin: 0 }}>{upMsg.text}</span>}
      <input ref={filePicker} type="file" multiple hidden
        onChange={(e) => { upload(Array.from(e.target.files ?? [])); e.target.value = ""; }} />
      <input ref={folderPicker} type="file" hidden {...{ webkitdirectory: "" }}
        onChange={(e) => { upload(Array.from(e.target.files ?? [])); e.target.value = ""; }} />
    </>
  );
}
