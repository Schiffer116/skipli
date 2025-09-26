import {
  Router,
  type NextFunction,
  type Request,
  type Response,
} from "express";
import { FieldValue } from "firebase-admin/firestore";

import { db, transporter } from "../index.js";
import cardRouter from "./card.js";

async function requireBoardMembership(
  req: any,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const { boardId } = req.params;
  if (!boardId) {
    throw new Error("Missing board id");
  }

  const email = req.email;
  const boardSnapshot = await db.collection("boards").doc(boardId).get();

  if (!boardSnapshot.exists) {
    res.status(404).end();
    return;
  }

  if (!boardSnapshot.data()?.members.includes(email)) {
    res.status(403).end();
    return;
  }

  req.boardSnapshot = boardSnapshot;
  next();
}

async function requireBoardOwnership(
  req: any,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const { email, boardSnapshot } = req;
  if (boardSnapshot.data()?.owner !== email) {
    res.status(403).end();
    return;
  }

  next();
}

const router: Router = Router();
router.use("/:boardId/cards", requireBoardMembership, cardRouter);

router.post("/", async (req: Request, res: Response) => {
  const { name, description } = req.body;
  const email = req.email;

  const board = await db.collection("boards").add({
    name,
    description,
    owner: email,
    members: [email],
  });

  res.status(201).json({
    id: board.id,
    name,
    description,
  });
});

router.get("/", async (req: Request, res: Response) => {
  const email = req.email;

  const boardsRef = db.collection("boards").select("name", "description");

  const boardsQuery = boardsRef.where("owner", "==", email);
  const boardSnapshot = await boardsQuery.get();
  const boards = boardSnapshot.docs.map((doc) => ({
    id: doc.id,
    ...doc.data(),
  }));

  const teamBoardsSnapshot = await boardsRef
    .where("members", "array-contains", email)
    .get();
  const teamBoards = teamBoardsSnapshot.docs.map((doc) => ({
    id: doc.id,
    ...doc.data(),
  }));

  res.json({ boards, teamBoards });
});

router.get(
  "/:boardId",
  requireBoardMembership,
  async (req: Request, res: Response) => {
    const { boardId } = req.params;
    const board = await db.collection("boards").doc(boardId!).get();
    const { name, description } = board.data()!;
    res.json({ id: boardId, name, description });
  },
);

router.put(
  "/:boardId",
  requireBoardMembership,
  requireBoardOwnership,
  async (req: Request, res: Response) => {
    const { boardId } = req.params;
    const { name, description } = req.body;
    await db.collection("boards").doc(boardId!).set(
      {
        name,
        description,
      },
      { merge: true },
    );

    res.json({ id: boardId, name, description });
  },
);

router.delete(
  "/:boardId",
  requireBoardMembership,
  requireBoardOwnership,
  async (req: Request, res: Response) => {
    const { boardId } = req.params;

    const boardRef = db.collection("boards").doc(boardId!);

    const batch = db.batch();

    const cardsQuery = await boardRef.collection("cards").get();
    cardsQuery.forEach(async (card) => {
      const taskQuery = await boardRef
        .collection("cards")
        .doc(card.id)
        .collection("tasks")
        .get();

      taskQuery.forEach(async (task) => {
        batch.delete(task.ref);
      });

      batch.delete(card.ref);
    });
    batch.delete(boardRef);

    await batch.commit();
    res.status(204).end();
  },
);

router.post(
  "/:boardId/invite",
  requireBoardMembership,
  requireBoardOwnership,
  async (req: Request, res: Response) => {
    const { boardId } = req.params;
    const { email } = req.body;

    const boardRef = db.collection("boards").doc(boardId!);
    const boardSnapshot = await boardRef.get();
    if (!boardSnapshot.exists) {
      return res.status(404).end();
    }

    const token = Math.random()
      .toString(36)
      .slice(2, 2 + 12);

    await boardRef.collection("invites").add({
      email,
      token,
    });

    await transporter.sendMail({
      from: "skipidili",
      to: email,
      subject: "You’ve been invited to join a board",
      text: `Click here to accept: http://localhost:5173/boards/${boardId}/invite/accept?token=${token}`,
    });

    res.status(200).end();
  },
);

router.post("/:boardId/invite/accept", async (req: Request, res: Response) => {
  const { boardId } = req.params;
  const { token } = req.body;
  const email = req.email;

  const boardRef = db.collection("boards").doc(boardId!);
  const inviteSnapshot = await boardRef
    .collection("invites")
    .where("token", "==", token)
    .where("email", "==", email)
    .get();

  if (!inviteSnapshot.docs.length) {
    return res.status(404).end();
  }

  boardRef.update({
    members: FieldValue.arrayUnion(email),
  });

  const batch = db.batch();
  inviteSnapshot.forEach((doc) => {
    doc.ref.delete();
  });
  batch.commit();

  res.status(200).end();
});

router.get(
  "/:boardId/members",
  requireBoardMembership,
  async (req: Request, res: Response) => {
    const { boardId } = req.params;

    const boardSnapshot = await db.collection("boards").doc(boardId!).get();

    if (!boardSnapshot.exists) {
      return res.status(404).end();
    }

    res.status(200).json(boardSnapshot.data()!.members);
  },
);

export default router;
