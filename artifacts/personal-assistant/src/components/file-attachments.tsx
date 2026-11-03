import { useCallback, useEffect, useRef, useState } from 'react';
import { Download, Loader2, Paperclip, Trash2, Upload } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { goApi, type FileAttachment } from '@/lib/go-api';

type Props =
  | { kind: 'note'; noteId: number }
  | { kind: 'task'; workspaceId: string; taskId: string };

const MAX_FILE_BYTES = 8 * 1024 * 1024;
const MAX_FILES = 20;

export function FileAttachments(props: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<FileAttachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const kind = props.kind;
  const noteId = props.kind === 'note' ? props.noteId : undefined;
  const workspaceId = props.kind === 'task' ? props.workspaceId : undefined;
  const taskId = props.kind === 'task' ? props.taskId : undefined;
  const refresh = useCallback(async () => {
    const result = kind === 'note'
      ? await goApi.listNoteAttachments(noteId!)
      : await goApi.listTaskAttachments(workspaceId!, taskId!);
    setFiles(result.attachments);
  }, [kind, noteId, workspaceId, taskId]);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const result = kind === 'note'
          ? await goApi.listNoteAttachments(noteId!)
          : await goApi.listTaskAttachments(workspaceId!, taskId!);
        if (active) setFiles(result.attachments);
      } catch {
        if (active) setError('Could not load attachments.');
      }
    };
    void load();
    return () => { active = false; };
  }, [kind, noteId, workspaceId, taskId]);

  const upload = async (selected: FileList | null) => {
    if (!selected?.length) return;
    setError('');
    const incoming = Array.from(selected);
    if (files.length + incoming.length > MAX_FILES) {
      setError(`An item can have up to ${MAX_FILES} files.`);
      return;
    }
    const tooLarge = incoming.find((file) => file.size < 1 || file.size > MAX_FILE_BYTES);
    if (tooLarge) {
      setError('Each file must be smaller than 8 MB.');
      return;
    }
    setBusy(true);
    try {
      for (const file of incoming) {
        if (props.kind === 'note') await goApi.uploadNoteAttachment(props.noteId, file);
        else await goApi.uploadTaskAttachment(props.workspaceId, props.taskId, file);
      }
      await refresh();
    } catch {
      setError('Some files could not be uploaded. Refresh the list and try again.');
      await refresh().catch(() => undefined);
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  };

  const remove = async (attachment: FileAttachment) => {
    setError('');
    setBusy(true);
    try {
      if (props.kind === 'note') await goApi.deleteNoteAttachment(props.noteId, attachment.id);
      else await goApi.deleteTaskAttachment(props.workspaceId, props.taskId, attachment.id);
      await refresh();
    } catch {
      setError('Could not remove this attachment.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="space-y-2" aria-label="Attachments">
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2 text-sm font-medium">
          <Paperclip className="size-4" aria-hidden="true" />
          Attachments ({files.length}/{MAX_FILES})
        </h3>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="sr-only"
          aria-label="Choose files to attach"
          disabled={busy || files.length >= MAX_FILES}
          onChange={(event) => void upload(event.currentTarget.files)}
        />
        <Button type="button" variant="outline" size="sm" disabled={busy || files.length >= MAX_FILES} onClick={() => inputRef.current?.click()}>
          {busy ? <Loader2 className="me-2 size-4 animate-spin" aria-hidden="true" /> : <Upload className="me-2 size-4" aria-hidden="true" />}
          Add files
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">Up to 20 files, 8 MB each. Files are private to this item.</p>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {files.length > 0 ? (
        <ul className="divide-y rounded-md border">
          {files.map((file) => (
            <li key={file.id} className="flex min-w-0 items-center gap-2 p-2">
              <span className="min-w-0 flex-1 truncate text-sm" title={file.fileName}>{file.fileName}</span>
              <span className="shrink-0 text-xs text-muted-foreground">{Math.max(1, Math.ceil(file.sizeBytes / 1024))} KB</span>
              <Button asChild variant="ghost" size="icon" className="size-8" aria-label={`Download ${file.fileName}`}>
              <a href={goApi.attachmentHref(file.downloadUrl)}><Download className="size-4" aria-hidden="true" /></a>
              </Button>
              <Button type="button" variant="ghost" size="icon" className="size-8" disabled={busy} onClick={() => void remove(file)} aria-label={`Remove ${file.fileName}`}>
                <Trash2 className="size-4" aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No attachments yet.</p>
      )}
    </section>
  );
}
