// Serialises faucet transfers so that only one is in flight at a time.
//
// Every transfer is signed with the faucet account's next sequence number. If two requests fetch the
// same sequence and broadcast together, one is rejected with a sequence mismatch and the client gets
// a failure for a request that was valid. Running transfers one after another in this process
// avoids that. The queue is per process: run one faucet instance per faucet account.

'use strict';

function createSequenceQueue() {
  let tail = Promise.resolve();
  let pending = 0;

  // run(task) starts task once every earlier task has settled, and resolves or rejects with its
  // result. A failed task does not block the tasks queued after it.
  function run(task) {
    pending += 1;
    const result = tail.then(() => task());
    tail = result.then(
      () => {
        pending -= 1;
      },
      () => {
        pending -= 1;
      },
    );
    return result;
  }

  return {
    run,
    /** Number of transfers queued or in flight. */
    get pending() {
      return pending;
    },
  };
}

module.exports = { createSequenceQueue };
