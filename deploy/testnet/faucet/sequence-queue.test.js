'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { createSequenceQueue } = require('./sequence-queue');

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

test('tasks never overlap, even when requested concurrently', async () => {
  const queue = createSequenceQueue();
  let running = 0;
  let maxRunning = 0;
  const order = [];

  const tasks = [0, 1, 2, 3, 4].map((i) =>
    queue.run(async () => {
      running += 1;
      maxRunning = Math.max(maxRunning, running);
      order.push(i);
      await sleep(5 + (i % 2) * 5);
      running -= 1;
      return i * 10;
    }),
  );

  const results = await Promise.all(tasks);
  assert.equal(maxRunning, 1);
  assert.deepEqual(order, [0, 1, 2, 3, 4], 'tasks run in the order they were queued');
  assert.deepEqual(results, [0, 10, 20, 30, 40]);
});

test('a failed task rejects its caller and does not block later tasks', async () => {
  const queue = createSequenceQueue();
  const failing = queue.run(async () => {
    throw new Error('sequence mismatch');
  });
  const following = queue.run(async () => 'sent');

  await assert.rejects(failing, /sequence mismatch/);
  assert.equal(await following, 'sent');
});

test('pending counts queued and in-flight tasks and returns to zero', async () => {
  const queue = createSequenceQueue();
  const first = queue.run(() => sleep(10));
  const second = queue.run(async () => {});
  assert.equal(queue.pending, 2);
  await Promise.all([first, second]);
  assert.equal(queue.pending, 0);
});

test('a task that rejects synchronously does not leave the queue stuck', async () => {
  const queue = createSequenceQueue();
  await assert.rejects(queue.run(() => {
    throw new TypeError('boom');
  }), TypeError);
  assert.equal(await queue.run(() => 42), 42);
  assert.equal(queue.pending, 0);
});
