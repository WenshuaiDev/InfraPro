import type { AvatarTarget } from '@infrapro/api-client';
import { AccountAvatar as AvatarView } from '@infrapro/ui/patterns/account-avatar';
import { useAvatarImage, type AvatarMetadata } from './use-avatar-image';

export function AccountAvatar({ name, metadata, target, className }: { name: string; metadata?: AvatarMetadata; target?: AvatarTarget; className?: string }) {
  const src = useAvatarImage(target, metadata);
  return <AvatarView name={name} src={src} className={className} />;
}
