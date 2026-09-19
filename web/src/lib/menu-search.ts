import { pinyin } from "pinyin-pro";
import type { NavItem } from "@/config/navigation";

type SearchText = { name: string; pinyin: string; initials: string };
export type MenuSearchItem = NavItem & {
  breadcrumb: string;
  titleIndex: SearchText;
  parentIndex: SearchText;
};

function normalize(value: string) {
  return value.toLowerCase().replace(/\s+/g, "");
}

function indexText(value: string): SearchText {
  return {
    name: normalize(value),
    pinyin: normalize(pinyin(value, { toneType: "none", v: true })),
    initials: normalize(pinyin(value, { pattern: "first", toneType: "none" })),
  };
}

export function buildMenuSearchIndex(items: NavItem[]): MenuSearchItem[] {
  const result: MenuSearchItem[] = [];
  const walk = (nodes: NavItem[], parents: string[]) => {
    nodes.forEach((item) => {
      if (item.children?.length) {
        walk(item.children, [...parents, item.label]);
      } else {
        result.push({
          ...item,
          breadcrumb: parents.join(" / "),
          titleIndex: indexText(item.label),
          parentIndex: indexText(parents.join(" ")),
        });
      }
    });
  };
  walk(items, []);
  return result;
}

export function searchMenus(items: MenuSearchItem[], query: string) {
  const keyword = normalize(query);
  if (!keyword) return items;

  const matches = (index: SearchText) =>
    Object.values(index).some((value) => value.includes(keyword));

  return items
    .map((item) => {
      const title = item.titleIndex;
      const score = title.name === keyword ? 0
        : title.name.startsWith(keyword) ? 1
        : title.name.includes(keyword) ? 2
        : matches(title) ? 3
        : matches(item.parentIndex) ? 4
        : -1;
      return { item, score };
    })
    .filter(({ score }) => score >= 0)
    .sort((a, b) => a.score - b.score)
    .map(({ item }) => item);
}
