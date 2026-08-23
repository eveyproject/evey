// outputMix maps each Web Audio OUTPUT channel to the decoder source channels to
// sum into it (each a [sourceIndex, gain] tap). It puts the decoder's layout
// into Web Audio's "speakers" order, and folds 7.1 down to 5.1: Web Audio only
// applies its speaker down-mix (the one that keeps the centre on a stereo output)
// up to 5.1 - an 8-channel buffer falls back to "discrete", which on a stereo
// device drops every channel past L/R, i.e. the dialogue. Folding to 5.1 keeps
// the browser's correct down-mix AND preserves the 7.1 back-channel content
// (mixed into the surrounds) instead of dropping it. Exported for unit testing.
//   5.1 decoder L,C,R,Ls,Rs,LFE          -> WA L,R,C,LFE,SL,SR
//   7.1 decoder L,R,C,LFE,BL,BR,SL,SR     -> WA 5.1, SL=(SL+BL)*.7071, SR likewise
export const FOLD_GAIN = Math.SQRT1_2;

export function outputMix(channels: number): Array<Array<[number, number]>> {
  if (channels === 6) {
    return [[[0, 1]], [[2, 1]], [[1, 1]], [[5, 1]], [[3, 1]], [[4, 1]]];
  }
  if (channels === 8) {
    // Both taps of a folded pair carry -3 dB: summing two channels at unity
    // gain made the 7.1 path peak at 3.12 (+9.9 dB) once the browser applied
    // its own un-normalised "speakers" fold on top, versus 2.41 for a native
    // 5.1 source. ffmpeg keeps the side channel at unity here because its
    // down-mix normalises afterwards; ours has nothing downstream that does.
    return [
      [[0, 1]],
      [[1, 1]],
      [[2, 1]],
      [[3, 1]],
      [[6, FOLD_GAIN], [4, FOLD_GAIN]],
      [[7, FOLD_GAIN], [5, FOLD_GAIN]],
    ];
  }
  return Array.from({ length: channels }, (_, i) => [[i, 1]] as Array<[number, number]>);
}
