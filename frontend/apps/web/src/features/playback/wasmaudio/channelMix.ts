// outputMix maps each Web Audio OUTPUT channel to the decoder source channels to
// sum into it (each a [sourceIndex, gain] tap). It puts the decoder's layout
// into Web Audio's "speakers" order, and folds 7.1 down to 5.1: Web Audio only
// applies its speaker down-mix (the one that keeps the centre on a stereo output)
// up to 5.1 - an 8-channel buffer falls back to "discrete", which on a stereo
// device drops every channel past L/R, i.e. the dialogue. Folding to 5.1 keeps
// the browser's correct down-mix AND preserves the 7.1 back-channel content
// (mixed into the surrounds) instead of dropping it. Exported for unit testing.
//   5.1 decoder L,C,R,Ls,Rs,LFE          -> WA L,R,C,LFE,SL,SR
//   7.1 decoder L,R,C,LFE,BL,BR,SL,SR     -> WA 5.1, SL=SL+BL, SR=SR+BR
export function outputMix(channels: number): Array<Array<[number, number]>> {
  if (channels === 6) {
    return [[[0, 1]], [[2, 1]], [[1, 1]], [[5, 1]], [[3, 1]], [[4, 1]]];
  }
  if (channels === 8) {
    return [[[0, 1]], [[1, 1]], [[2, 1]], [[3, 1]], [[6, 1], [4, 1]], [[7, 1], [5, 1]]];
  }
  return Array.from({ length: channels }, (_, i) => [[i, 1]] as Array<[number, number]>);
}
