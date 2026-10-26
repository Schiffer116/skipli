import { KeyboardSensor } from "@dnd-kit/core";

// Key presses inside a portaled dialog still bubble through React to the
// draggable that rendered it, and dnd-kit's sensor would take Space as "pick
// up", eating spaces typed into the dialog. Only start from the draggable
// itself.
export class OwnKeyboardSensor extends KeyboardSensor {
  static activators = KeyboardSensor.activators.map((activator) => ({
    ...activator,
    handler: (...args: Parameters<typeof activator.handler>) =>
      args[0].target === args[0].currentTarget && activator.handler(...args),
  }));
}

// For draggables that are links: Enter has to keep following the link, so
// only Space picks one up.
export class SpaceKeyboardSensor extends OwnKeyboardSensor {
  static activators = OwnKeyboardSensor.activators.map((activator) => ({
    ...activator,
    handler: (...args: Parameters<typeof activator.handler>) =>
      args[0].code === "Space" && activator.handler(...args),
  }));
}
