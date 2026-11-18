import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Camera, ContactRound, RotateCcw } from 'lucide-react';
import { toast } from 'sonner';
import { Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { date } from '../auth/forms';
import { AccountAvatar } from './account-avatar';
import { useOperation } from './use-operation';

type Selection = { image: HTMLImageElement; url: string; revision: string };
type Crop = { x: number; y: number; zoom: number };
const initialCrop: Crop = { x: 0.5, y: 0.5, zoom: 1 };
const clamp = (value: number) => Math.min(1, Math.max(0, value));

export function ProfileIdentityCard() {
  const { account, can, client, reload } = useIdentity();
  const cache = useQueryClient();
  const input = useRef<HTMLInputElement>(null), trigger = useRef<HTMLButtonElement>(null);
  const generation = useRef(0);
  const [selection, setSelection] = useState<Selection>();
  const [reading, setReading] = useState(false), [error, setError] = useState('');
  const [removeOpen, setRemoveOpen] = useState(false);
  const removeRevision = useRef('0');
  useEffect(() => () => { generation.current++; }, []);
  useEffect(() => () => { if (selection) URL.revokeObjectURL(selection.url); }, [selection]);
  const changed = async (message: string) => {
    await reload();
    await cache.invalidateQueries({ predicate: (query) => ['avatar', 'account', 'accounts', 'access', 'tenant', 'project'].includes(String(query.queryKey[0])) });
    toast.success(message);
  };
  const remove = useOperation({ mutationFn: () => client.removeAvatar(removeRevision.current), onSuccess: async () => { setRemoveOpen(false); await changed('头像已移除。'); } });
  if (!account) return null;
  const choose = async (file?: File) => {
    if (!file) return;
    const version = ++generation.current;
    setError('');
    if (!['image/jpeg', 'image/png'].includes(file.type) || file.size === 0 || file.size > 5 * 1024 * 1024) { setError('请选择 JPG 或 PNG 图片，大小不超过 5 MB。'); return; }
    setReading(true);
    const url = URL.createObjectURL(file), image = new Image();
    try {
      image.src = url; await image.decode();
      if (image.naturalWidth * image.naturalHeight > 25_000_000) throw new Error('图片不能超过 2500 万像素。');
      if (version !== generation.current) { URL.revokeObjectURL(url); return; }
      setSelection({ image, url, revision: account.avatar_revision ?? '0' });
    } catch (cause) {
      URL.revokeObjectURL(url);
      if (version === generation.current) setError(cause instanceof Error && cause.message.includes('像素') ? cause.message : '图片无法读取，请选择有效的 JPG 或 PNG 图片。');
    } finally { if (version === generation.current) setReading(false); }
  };
  return <section className="account-cover flex min-h-72 flex-col items-start justify-between gap-7" aria-labelledby="profile-card-title">
    <h2 id="profile-card-title" className="account-cover-muted flex items-center gap-2 text-sm font-medium"><ContactRound className="size-4" aria-hidden="true" />账号名片</h2>
    <div className="grid min-w-0 gap-3">
      <AccountAvatar name={account.display_name} metadata={account} target={{ kind: 'self' }} className="size-16 rounded-xl text-xl text-foreground ring-2 ring-white/35" />
      <div><p className="text-3xl font-semibold tracking-tight">{account.display_name}</p><p className="account-cover-muted mt-1 break-all text-sm"><span className="sr-only">用户名 </span>@{account.username}</p></div>
      {can(P.AccountProfileUpdate) && <div>
        <input ref={input} type="file" accept="image/jpeg,image/png" aria-label="选择头像图片" className="sr-only" tabIndex={-1} onChange={event => { void choose(event.target.files?.[0]); event.target.value = ''; }} />
        <div className="flex flex-wrap items-center gap-x-5">
          <button ref={trigger} type="button" className="inline-flex min-h-11 cursor-pointer items-center gap-2 text-sm font-medium underline decoration-white/40 underline-offset-4 hover:decoration-white disabled:opacity-60" disabled={reading || remove.isPending} onClick={() => input.current?.click()}><Camera className="size-4" aria-hidden="true" />{reading ? '正在读取…' : '更换头像'}</button>
          {account.has_avatar && <button type="button" className="account-cover-muted min-h-11 cursor-pointer text-sm hover:underline" disabled={reading || remove.isPending} onClick={() => { remove.reset(); removeRevision.current = account.avatar_revision ?? '0'; setRemoveOpen(true); }}>移除头像</button>}
        </div>
        <p className="account-cover-muted text-xs">JPG 或 PNG，最大 5 MB</p>
      </div>}
      {error && <p role="alert" className="rounded-md bg-white p-2 text-sm text-red-800">{error}</p>}
    </div>
    <div className="account-cover-muted w-full border-t border-white/20 pt-4 text-xs">创建于 <time dateTime={account.created_at}>{date(account.created_at)}</time></div>
    {selection && <AvatarCrop image={selection.image} writeImage={blob => client.updateAvatar(blob, selection.revision)} close={() => setSelection(undefined)} saved={() => changed('头像已保存。')} returnFocus={() => trigger.current?.focus()} />}
    <Dialog open={removeOpen} onOpenChange={(open) => { if (!remove.isPending) setRemoveOpen(open); }}><DialogContent showCloseButton={!remove.isPending} onCloseAutoFocus={(event) => { event.preventDefault(); trigger.current?.focus(); }}>
      <DialogHeader><DialogTitle>移除头像</DialogTitle><DialogDescription>移除后，各个工作空间将使用姓名文字作为默认头像。</DialogDescription></DialogHeader>
      {remove.error && <FormNotice error>{remove.error.message}</FormNotice>}
      <DialogFooter><Button variant="outline" disabled={remove.isPending} onClick={() => setRemoveOpen(false)}>取消</Button><Button disabled={remove.isPending} onClick={() => remove.mutate()}>{remove.isPending ? '正在移除…' : '确认移除'}</Button></DialogFooter>
    </DialogContent></Dialog>
  </section>;
}

function AvatarCrop({ image, writeImage, close, saved, returnFocus }: { image: HTMLImageElement; writeImage: (blob: Blob) => Promise<unknown>; close: () => void; saved?: () => void | Promise<void>; returnFocus: () => void }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const drag = useRef<{ x: number; y: number; crop: Crop } | undefined>(undefined);
  const [crop, setCrop] = useState(initialCrop);
  const edge = Math.min(image.naturalWidth, image.naturalHeight) / crop.zoom;
  const drawPreview = useCallback((element: HTMLCanvasElement | null) => {
    canvas.current = element;
    const context = element?.getContext('2d');
    if (!context) return;
    context.clearRect(0, 0, 256, 256);
    context.imageSmoothingEnabled = true; context.imageSmoothingQuality = 'high';
    context.drawImage(image, (image.naturalWidth - edge) * crop.x, (image.naturalHeight - edge) * crop.y, edge, edge, 0, 0, 256, 256);
  }, [image, edge, crop]);
  const save = useOperation({ mutationFn: async () => {
    const output = canvas.current;
    if (!output) throw new Error('图片预览尚未就绪。');
    const blob = await new Promise<Blob>((resolve, reject) => output.toBlob((value) => value ? resolve(value) : reject(new Error('图片处理失败，请重新选择。')), 'image/png'));
    return writeImage(blob);
  }, onSuccess: async () => { await saved?.(); close(); } });
  return <Dialog open onOpenChange={(open) => { if (!open && !save.isPending) close(); }}><DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-md" showCloseButton={!save.isPending} onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus(); }}>
    <DialogHeader><DialogTitle>调整头像</DialogTitle><DialogDescription>拖动图片调整位置，或使用下方滑块。预览即保存后的效果。</DialogDescription></DialogHeader>
    <div className="mx-auto max-w-full rounded-lg border bg-muted/30 p-1">
      <canvas ref={drawPreview} width={256} height={256} role="img" aria-label="头像裁剪预览，可用方向键调整位置" tabIndex={0} className="aspect-square w-64 max-w-full touch-none cursor-grab rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring active:cursor-grabbing"
        onKeyDown={(event) => { const step = event.shiftKey ? 0.1 : 0.02; if (save.isPending || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return; event.preventDefault(); setCrop((old) => ({ ...old, x: clamp(old.x + (event.key === 'ArrowLeft' ? -step : event.key === 'ArrowRight' ? step : 0)), y: clamp(old.y + (event.key === 'ArrowUp' ? -step : event.key === 'ArrowDown' ? step : 0)) })); }}
        onPointerDown={(event) => { if (save.isPending) return; event.currentTarget.setPointerCapture(event.pointerId); drag.current = { x: event.clientX, y: event.clientY, crop }; }}
        onPointerUp={() => { drag.current = undefined; }} onPointerCancel={() => { drag.current = undefined; }}
        onPointerMove={(event) => { const start = drag.current; if (!start || save.isPending) return; const scale = edge / event.currentTarget.getBoundingClientRect().width; setCrop({ ...start.crop, x: clamp(start.crop.x - (event.clientX - start.x) * scale / Math.max(1, image.naturalWidth - edge)), y: clamp(start.crop.y - (event.clientY - start.y) * scale / Math.max(1, image.naturalHeight - edge)) }); }} />
    </div>
    <fieldset disabled={save.isPending} className="space-y-3"><legend className="sr-only">裁剪控制</legend>
      {([{ key: 'zoom', label: '缩放', min: 1, max: 4, step: 0.01 }, { key: 'x', label: '水平位置', min: 0, max: 1, step: 0.01 }, { key: 'y', label: '垂直位置', min: 0, max: 1, step: 0.01 }] as const).map((control) => <label key={control.key} className="flex min-h-11 items-center gap-4 text-sm"><span className="w-16 shrink-0">{control.label}</span><input type="range" aria-label={control.label} className="h-6 min-w-0 flex-1 accent-primary" min={control.min} max={control.max} step={control.step} value={crop[control.key]} onChange={(event) => setCrop((old) => ({ ...old, [control.key]: Number(event.target.value) }))} /><output className="w-10 text-right text-xs tabular-nums">{Math.round(crop[control.key] * 100)}%</output></label>)}
      <Button type="button" variant="ghost" size="sm" onClick={() => setCrop(initialCrop)}><RotateCcw aria-hidden="true" />重置裁剪</Button>
    </fieldset>
    {save.error && <FormNotice error>{save.error.message}</FormNotice>}
    <DialogFooter><Button variant="outline" disabled={save.isPending} onClick={close}>取消</Button><Button disabled={save.isPending} onClick={() => save.mutate()}>{save.isPending ? '正在保存…' : '保存头像'}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
