import { useEffect, useMemo } from 'react';
import { hotkeysCoreFeature, selectionFeature, syncDataLoaderFeature } from '@headless-tree/core';
import { useTree } from '@headless-tree/react';
import { useLocation } from 'wouter';
import { Tree, TreeItem, TreeItemLabel } from '@/components/reui/tree';

type SettingsItem = { name: string; children?: string[]; href?: string };

const settingsItems: Record<string, SettingsItem> = {
  settings: { name: 'Settings', children: ['account', 'space', 'credits'] },
  account: { name: 'Account', children: ['profile', 'security', 'privacy'] },
  profile: { name: 'Profile', href: '/settings/account/profile' },
  security: { name: 'Security', href: '/settings/account/security' },
  privacy: { name: 'Privacy', href: '/settings/account/privacy' },
  space: { name: 'Space', children: ['details', 'people', 'notifications'] },
  details: { name: 'Details', href: '/settings/space/details' },
  people: { name: 'People', href: '/settings/space/members' },
  notifications: { name: 'Notifications', href: '/settings/space/notifications' },
  credits: { name: 'Credits', children: ['usage', 'billing'] },
  usage: { name: 'Usage', href: '/settings/space/usage' },
  billing: { name: 'Billing', href: '/settings/space/billing' },
};

const pathToId: Record<string, string> = Object.fromEntries(
  Object.entries(settingsItems)
    .filter(([, item]) => item.href)
    .map(([id, item]) => [item.href as string, id]),
);

export function SpaceSettingsNavigation() {
  const [location, setLocation] = useLocation();
  const selectedId = pathToId[location.split('?')[0]] ?? 'profile';
  const treeData = useMemo(() => settingsItems, []);
  const tree = useTree<SettingsItem>({
    rootItemId: 'settings',
    indent: 18,
    initialState: {
      expandedItems: ['settings', 'account', 'space', 'credits'],
      selectedItems: [selectedId],
    },
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => Boolean(item.getItemData().children?.length),
    dataLoader: {
      getItem: (itemId) => treeData[itemId],
      getChildren: (itemId) => treeData[itemId]?.children ?? [],
    },
    features: [syncDataLoaderFeature, selectionFeature, hotkeysCoreFeature],
  });

  useEffect(() => {
    tree.setSelectedItems([selectedId]);
  }, [selectedId, tree]);

  return (
    <nav aria-label="Settings" className="settings-tree-shell rounded-2xl border border-border/70 bg-card/60 p-3">
      <p className="px-2 pb-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
        Preferences
      </p>
      <Tree
        tree={tree}
        indent={18}
        className="settings-tree"
        aria-label="Settings destinations"
      >
        {tree.getItems().map((item) => {
          const id = item.getId();
          const destination = treeData[id]?.href;
          const active = id === selectedId;
          return (
            <TreeItem
              key={id}
              item={item}
              className="settings-tree-item"
              onItemClick={() => {
                if (destination && destination !== location) setLocation(destination);
              }}
            >
              <TreeItemLabel
                className={`settings-tree-label ${active ? 'is-current' : ''}`}
                aria-current={active ? 'page' : undefined}
              >
                <span className="pointer-events-none">{item.getItemName()}</span>
              </TreeItemLabel>
            </TreeItem>
          );
        })}
      </Tree>
    </nav>
  );
}
