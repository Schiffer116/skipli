import { Router, type Request, type Response } from "express";

import { db } from "../index.js";
import taskRouter from "./task.js";
import { ORDER_GAP } from "../index.js";

const router: Router = Router({ mergeParams: true });
router.use("/:cardId/tasks", taskRouter);

router.get("/", async (req: Request, res: Response) => {
  const { boardId } = req.params;

  const query = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .orderBy("order")
    .get();

  const cards =
    query.docs.map((doc) => {
      const { name, description } = doc.data()!;
      return { id: doc.id, name, description };
    }) || [];
  res.json(cards);
});

router.post("/", async (req: Request, res: Response) => {
  const { boardId } = req.params;
  const { name, description, createdAt } = req.body;

  const lastCard = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .orderBy("order", "desc")
    .limit(1)
    .get();

  const lastOrder: number = lastCard.docs[0]?.data()!.order ?? 0;

  const card = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .add({
      name,
      description,
      createdAt,
      order: lastOrder + ORDER_GAP,
    });

  res.status(201).json({
    id: card.id,
    name,
    description,
  });
});

router.get("/:id", async (req: Request, res: Response) => {
  const { boardId, id } = req.params;
  const card = await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(id!)
    .get();

  const { name, description } = card.data()!;

  res.json({
    id: card.id,
    name,
    description,
  });
});

router.put("/:id", async (req: Request, res: Response) => {
  const { boardId, id } = req.params;
  const { name, description } = req.body;

  await db.collection("boards").doc(boardId!).collection("cards").doc(id!).set(
    {
      name,
      description,
    },
    { merge: true },
  );

  res.json({ id, name, description });
});

router.patch("/:id", async (req: Request, res: Response) => {
  const { boardId, id } = req.params;
  const { beforeId, afterId } = req.body;

  if (!beforeId && !afterId) {
    return res.status(200).end();
  }

  const [beforeCard, afterCard] = await Promise.all([
    beforeId &&
    db
      .collection("boards")
      .doc(boardId!)
      .collection("cards")
      .doc(beforeId!)
      .get(),
    afterId &&
    db
      .collection("boards")
      .doc(boardId!)
      .collection("cards")
      .doc(afterId!)
      .get(),
  ]);

  const validBefore = !beforeId || beforeCard.exists;
  const validAfter = !afterId || afterCard.exists;
  if (!validBefore || !validAfter) {
    return res.status(404).end();
  }

  const afterOrder = afterCard?.data()?.order;
  const beforeOrder = beforeCard?.data()?.order;

  const newOrder =
    beforeOrder === undefined
      ? afterOrder - ORDER_GAP
      : afterOrder === undefined
        ? beforeOrder + ORDER_GAP
        : beforeOrder + (afterOrder - beforeOrder) / 2;

  await db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(id!)
    .update({
      order: newOrder,
    });

  res.status(200).end();
});

router.delete("/:id", async (req: Request, res: Response) => {
  const { boardId, id } = req.params;
  const card = db
    .collection("boards")
    .doc(boardId!)
    .collection("cards")
    .doc(id!);

  const batch = db.batch();
  const tasks = await card.collection("tasks").get();

  tasks.forEach(async (task) => {
    batch.delete(task.ref);
  });

  batch.delete(card);

  await batch.commit();
  res.status(204).end();
});

export default router;
