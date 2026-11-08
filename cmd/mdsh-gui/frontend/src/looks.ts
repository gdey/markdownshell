// The looks, from their folders in frontend/looks: looks.gen.ts is made
// from each look.css's front matter by go generate. See docs/looks.md.
import { generated } from "./looks.gen.js";

// Look is what the pages know of a look.
export interface Look {
  // id is the look setting's value and the root's data-look; Plain's is
  // empty.
  id: string;
  name: string;
  // description is shown under Settings › Look's menu.
  description: string;
  // textSize is the look's own text size, in percent.
  textSize: number;
  // texture, if any, names the texture switches the look draws: the
  // group's label and each switch's, empty for a switch it doesn't draw.
  texture: { label: string; sprockets: string; rules: string; shadows: string } | null;
}

// looks are all the looks, in the menu's order; Plain is first.
export const looks: readonly Look[] = generated;

// lookFor is the look with id, or Plain for an id no look has.
export function lookFor(id: string): Look {
  return looks.find((l) => l.id === id) ?? looks[0];
}
