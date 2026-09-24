/**
 * Whether "who is watching" walks straight through: a single profile, or a
 * household with no kids profile, has nothing to protect - unless the viewer
 * asked to switch, which must always show the picker.
 */
export function walksThroughProfilePicker(input: { asked: boolean; profileCount: number; hasKidsProfile: boolean }): boolean {
  if (input.asked || input.profileCount === 0) return false;
  return input.profileCount === 1 || !input.hasKidsProfile;
}
