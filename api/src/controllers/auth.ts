import { Router, type Request, type Response } from "express";
import jwt from "jsonwebtoken";

import { db, requireAuth, transporter } from "../index.js";

const router: Router = Router();

router.get("/email", requireAuth, async (req: Request, res: Response) => {
  const email = req.email;
  return res.status(200).json({ email });
});

router.post("/email", async (req: Request, res: Response) => {
  const { email } = req.body;

  const code = Math.random()
    .toString(36)
    .slice(2, 2 + 6);

  await db.collection("verification").doc(email).set({ code }, { merge: true });

  await transporter.sendMail({
    from: "skipli",
    to: email,
    subject: `${code} is your verification code`,
    text: `${code} is your verification code`,
  });

  return res.status(200).end();
});

router.post("/verify", async (req: Request, res: Response) => {
  const { email, code } = req.body;

  const verificationSnapshot = await db
    .collection("verification")
    .doc(email)
    .get();

  if (!verificationSnapshot.exists) {
    return res.status(401).json({
      message: "Invalid email or verification code",
    });
  }

  if (!code || verificationSnapshot.data()?.code !== code) {
    return res.status(401).json({
      message: "Invalid email or verification code",
    });
  }

  await verificationSnapshot.ref.delete();

  const accessToken = jwt.sign({ email }, process.env.JWT_SECRET!, {
    expiresIn: "1d",
  });

  res.json({ accessToken });
});

export default router;
