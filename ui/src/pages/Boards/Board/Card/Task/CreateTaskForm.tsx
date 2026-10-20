import { useContext } from "react";
import { useParams } from "react-router";

import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

import { BoardContext } from "../../BoardContext";
import useEscape from "@/hooks/useEscape";
import { newId } from "@/offline/ids";

type TaskFormProps = {
  cardId: string;
};

export default function CreateTaskForm(props: TaskFormProps) {
  const { cardId } = props;
  const { make, setCreateTaskFormId } = useContext(BoardContext)!;
  useEscape(() => setCreateTaskFormId(null));

  const boardId = useParams().boardId!;

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    if (!name) {
      return;
    }

    e.currentTarget.reset();

    void make({ type: "createTask", boardId, cardId, taskId: newId(), name });
  };

  return (
    <form className="w-full" onSubmit={handleSubmit} autoComplete="off">
      <div className="space-y-3 rounded-md w-full">
        <Input
          name="name"
          defaultValue=""
          placeholder="Name"
          autoFocus
          className="w-full h-10 rounded-md"
        />
        <div className="flex gap-2">
          <Button
            size="sm"
            className="bg-primary hover:bg-primary/90"
            type="submit"
          >
            Create
          </Button>
          <Button
            variant="ghost"
            size="sm"
            type="button"
            onClick={() => {
              setCreateTaskFormId(null);
            }}
          >
            Cancel
          </Button>
        </div>
      </div>
    </form>
  );
}
