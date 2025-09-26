import { Router, type Request, type Response } from "express";
import { db, ORDER_GAP } from "../index.js";

const router: Router = Router({ mergeParams: true });

router.get("/", async (req: Request, res: Response) => {
  const { boardId, cardId } = req.params;

  const query = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .orderBy("order")
    .get();

  const tasks =
    query.docs.map((doc) => ({
      id: doc.id,
      cardId,
      ...doc.data(),
    })) || [];
  res.json(tasks);
});

router.post("/", async (req: Request, res: Response) => {
  const { boardId, cardId } = req.params;
  const email = req.email;
  const { name } = req.body;

  const lastTask = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .orderBy("order", "desc")
    .limit(1)
    .get();

  const lastOrder: number = lastTask.docs[0]?.data()!.order ?? 0;

  const taskRef = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .add({
      name,
      description: "",
      status: "pending",
      order: lastOrder + ORDER_GAP,
    });

  res.status(201).json({
    id: taskRef.id,
    cardId,
    owner: email,
    name,
    description: "",
    status: "pending",
  });
});

router.get("/:id", async (req: Request, res: Response) => {
  const { boardId, cardId, id } = req.params;
  const taskRef = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .doc(id!)
    .get();

  res.json({
    id,
    cardId: cardId,
    ...taskRef.data(),
  });
});

router.put("/:id", async (req: Request, res: Response) => {
  const { boardId, cardId, id } = req.params;
  const { name, description, status } = req.body;

  await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .doc(id!)
    .set(
      {
        name,
        description,
        status,
      },
      { merge: true },
    );

  res.json({ id, name, description, status });
});

router.patch("/:id", async (req: Request, res: Response) => {
  const { boardId, cardId, id } = req.params;
  const { newCardId, beforeId, afterId } = req.body;

  const taskRef = db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .doc(id!);

  const task = await taskRef.get();
  if (!task.exists) {
    return res.status(404).json({ message: "Task doesn't exist" });
  }

  taskRef.delete();

  const [beforeTask, afterTask] = await Promise.all([
    beforeId &&
    db
      .collection("boards")
      .doc(boardId!)
      .collection("cards")
      .doc(newCardId!)
      .collection("tasks")
      .doc(beforeId!)
      .get(),
    afterId &&
    db
      .collection("boards")
      .doc(boardId!)
      .collection("cards")
      .doc(newCardId!)
      .collection("tasks")
      .doc(afterId!)
      .get(),
  ]);

  const validBefore = !beforeId || beforeTask.exists;
  const validAfter = !afterId || afterTask.exists;
  if (!validBefore || !validAfter) {
    return res.status(404).end();
  }
  const beforeOrder = beforeTask?.data()?.order;
  const afterOrder = afterTask?.data()?.order;

  const newOrder =
    beforeOrder === undefined && afterOrder === undefined
      ? 0
      : beforeOrder === undefined
        ? afterOrder - ORDER_GAP
        : afterOrder === undefined
          ? beforeOrder + ORDER_GAP
          : beforeOrder + (afterOrder - beforeOrder) / 2;

  await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(newCardId)
    .collection("tasks")
    .doc(id!)
    .set({
      ...task.data(),
      order: newOrder,
    });

  res.status(200).end();
});

router.delete("/:id", async (req: Request, res: Response) => {
  const { boardId, cardId, id } = req.params;
  await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(cardId!)
    .collection("tasks")
    .doc(id!)
    .delete();

  res.status(204).end();
});

export default router;
