import { useQueryClient } from '@tanstack/react-query';
import type { EngineeringCategory } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from './use-request-meta';

type Change = { category: EngineeringCategory; reason: string; remove?: boolean };

export function useCategoryMutation(done: (key: string, deleted: boolean) => void) {
  const { account, client } = useIdentity();
  const cache = useQueryClient();
  const meta = useRequestMeta();
  return useOperation({
    mutationFn: ({ category, reason, remove }: Change) => {
      const explanation = reason.trim();
      if (!explanation) throw new Error('请填写变更原因。');
      const { key, revision, name, description } = category;
      if (remove) {
        if (!revision || category.used) throw new Error('只能删除未引用的类别。');
        return client.engineeringDeleteCategory(key, { revision, ...meta('delete-category', { key, revision }, explanation) });
      }
      const input = { key, revision, name, description };
      return client.engineeringSaveCategory({ ...input, ...meta('save-category', input, explanation) });
    },
    onSuccess: async (result, change) => {
      await cache.invalidateQueries({ queryKey: ['platform', account?.id, 'engineering', 'categories'] });
      done(result.definition_id, Boolean(change.remove));
    },
  });
}
