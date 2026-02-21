'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useMutation, useQuery } from 'convex/react';
import { toast } from 'sonner';
import { api } from '@/convex/_generated/api';
import type { Id } from '@/convex/_generated/dataModel';
import { useConceptActions } from '@/hooks/use-concept-actions';
import { useInstantFeedback } from '@/hooks/use-instant-feedback';
import { useQuizInteractions } from '@/hooks/use-quiz-interactions';
import { useReviewFlow } from '@/hooks/use-review-flow';
import { useUnifiedEdit } from '@/hooks/use-unified-edit';
import type { QuestionType } from '@/lib/unified-edit-validation';

type ReviewFlowState = ReturnType<typeof useReviewFlow>;
type ReviewQuestion = NonNullable<ReviewFlowState['question']>;
type UnifiedEditState = ReturnType<typeof useUnifiedEdit>;

type SessionFeedbackState = {
  showFeedback: boolean;
  nextReviewInfo: {
    nextReview: Date | null;
    scheduledDays: number;
  } | null;
};

export interface ReviewSessionContextValue {
  phase: ReviewFlowState['phase'];
  question: ReviewFlowState['question'];
  conceptTitle: ReviewFlowState['conceptTitle'];
  conceptId: ReviewFlowState['conceptId'];
  phrasingId: ReviewFlowState['phrasingId'];
  phrasingIndex: ReviewFlowState['phrasingIndex'];
  totalPhrasings: ReviewFlowState['totalPhrasings'];
  selectionReason: ReviewFlowState['selectionReason'];
  interactions: ReviewFlowState['interactions'];
  isTransitioning: ReviewFlowState['isTransitioning'];
  conceptFsrs: ReviewFlowState['conceptFsrs'];
  selectedAnswer: string;
  feedbackState: SessionFeedbackState;
  hasAnsweredCurrentQuestion: boolean;
  currentInteractionId: Id<'interactions'> | null;
  userFeedback: 'helpful' | 'unhelpful' | null;
  displayConceptTitle: string;
  displayQuestion: ReviewQuestion | null;
  phrasingPositionLabel: string | null;
  selectionReasonLabel: string | null;
  instantFeedback: ReturnType<typeof useInstantFeedback>['feedbackState'];
  skipAnnouncement: string;
  cachedDueCount: number;
  unifiedEdit: UnifiedEditState;
  handleAnswerSelect: (answer: string) => void;
  handleSubmit: () => void;
  handleNext: () => void;
  handleSkip: () => void;
  handleArchivePhrasing: () => Promise<void>;
  handleArchiveConcept: () => Promise<void>;
  handleUserFeedback: (feedbackType: 'helpful' | 'unhelpful') => Promise<void>;
  handleStartInlineEdit: () => void;
  handleArchiveViaShortcut: () => void;
  handleQuestionDisplayReset: () => void;
}

export const ReviewSessionContext = createContext<ReviewSessionContextValue | null>(null);

export function ReviewSessionProvider({ children }: { children: ReactNode }) {
  const {
    phase,
    question,
    conceptTitle,
    conceptId,
    phrasingId,
    phrasingIndex,
    totalPhrasings,
    selectionReason,
    interactions,
    isTransitioning,
    conceptFsrs,
    skippedCount: _skippedCount,
    handlers,
  } = useReviewFlow();

  const {
    feedbackState: instantFeedback,
    showFeedback: showInstantFeedback,
    clearFeedback: clearInstantFeedback,
  } = useInstantFeedback();

  const isMountedRef = useRef(true);
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const prevPhrasingRef = useRef<string | null>(null);
  const [selectedAnswer, setSelectedAnswer] = useState('');
  const [feedbackState, setFeedbackState] = useState<SessionFeedbackState>({
    showFeedback: false,
    nextReviewInfo: null,
  });
  const [hasAnsweredCurrentQuestion, setHasAnsweredCurrentQuestion] = useState(false);
  const { trackAnswer } = useQuizInteractions();
  const recordFeedbackMutation = useMutation(api.concepts.recordFeedback);
  const [sessionId] = useState(() => Math.random().toString(36).substring(7));
  const [questionStartTime, setQuestionStartTime] = useState(() => Date.now());

  const [currentInteractionId, setCurrentInteractionId] = useState<Id<'interactions'> | null>(null);
  const [userFeedback, setUserFeedback] = useState<'helpful' | 'unhelpful' | null>(null);

  const selectionReasonDescriptions: Record<string, string> = {
    canonical: 'Your preferred phrasing',
    'least-seen': 'Least practiced',
    random: 'Random rotation',
    none: 'Default selection',
  };

  const hasPhrasingPosition =
    typeof phrasingIndex === 'number' &&
    typeof totalPhrasings === 'number' &&
    phrasingIndex > 0 &&
    totalPhrasings > 0;
  const phrasingPositionLabel = hasPhrasingPosition
    ? `Phrasing ${phrasingIndex} of ${totalPhrasings}`
    : null;
  const selectionReasonLabel = selectionReason
    ? (selectionReasonDescriptions[selectionReason] ?? 'Active phrasing')
    : null;

  const dueCountData = useQuery(api.concepts.getConceptsDueCount);
  const [cachedDueCount, setCachedDueCount] = useState(0);
  useEffect(() => {
    if (dueCountData !== undefined) {
      setCachedDueCount(dueCountData.conceptsDue);
    }
  }, [dueCountData]);

  const conceptActions = useConceptActions({ conceptId: conceptId ?? '' });

  const [optimisticConcept, setOptimisticConcept] = useState<{ title: string } | null>(null);
  const [optimisticPhrasing, setOptimisticPhrasing] = useState<{
    question: string;
    correctAnswer: string;
    explanation: string;
    options: string[];
  } | null>(null);

  const questionType: QuestionType =
    question?.type === 'true-false' ? 'true-false' : 'multiple-choice';

  const unifiedEdit = useUnifiedEdit(
    {
      conceptTitle: conceptTitle ?? '',
      question: question?.question ?? '',
      correctAnswer: question?.correctAnswer ?? '',
      explanation: question?.explanation ?? '',
      options: question?.options ?? [],
    },
    async (data) => {
      if (!conceptId) return;
      await conceptActions.editConcept(data);
      const optimisticData = { title: data.title };
      setOptimisticConcept(optimisticData);
      return optimisticData;
    },
    async (data) => {
      if (!phrasingId) return;
      const updated = await conceptActions.editPhrasing({
        phrasingId,
        question: data.question,
        correctAnswer: data.correctAnswer,
        explanation: data.explanation,
        options: data.options,
      });

      if (updated) {
        const optimisticData = {
          question: updated.question,
          correctAnswer: updated.correctAnswer ?? '',
          explanation: updated.explanation ?? '',
          options: updated.options ?? [],
        };
        setOptimisticPhrasing(optimisticData);
        return optimisticData;
      }
    },
    questionType
  );

  useEffect(() => {
    if (optimisticConcept && conceptTitle) {
      const conceptMatches = conceptTitle === optimisticConcept.title;
      if (conceptMatches) {
        setOptimisticConcept(null);
      }
    }
  }, [optimisticConcept, conceptTitle]);

  useEffect(() => {
    if (optimisticPhrasing && question) {
      const questionsMatch =
        question.question === optimisticPhrasing.question &&
        question.correctAnswer === optimisticPhrasing.correctAnswer &&
        question.explanation === optimisticPhrasing.explanation &&
        JSON.stringify(question.options) === JSON.stringify(optimisticPhrasing.options);

      if (questionsMatch) {
        setOptimisticPhrasing(null);
      }
    }
  }, [optimisticPhrasing, question]);

  const displayConceptTitle = useMemo(() => {
    if (unifiedEdit.isEditing) {
      return unifiedEdit.localData.conceptTitle;
    }
    if (optimisticConcept) {
      return optimisticConcept.title;
    }
    return conceptTitle ?? '';
  }, [unifiedEdit.isEditing, unifiedEdit.localData.conceptTitle, optimisticConcept, conceptTitle]);

  const displayQuestion = useMemo(() => {
    if (!question) return null;

    if (unifiedEdit.isEditing) {
      return {
        ...question,
        question: unifiedEdit.localData.question,
        correctAnswer: unifiedEdit.localData.correctAnswer,
        explanation: unifiedEdit.localData.explanation,
        options: unifiedEdit.localData.options,
      };
    }

    if (optimisticPhrasing) {
      return {
        ...question,
        question: optimisticPhrasing.question,
        correctAnswer: optimisticPhrasing.correctAnswer,
        explanation: optimisticPhrasing.explanation,
        options: optimisticPhrasing.options,
      };
    }

    return question;
  }, [question, optimisticPhrasing, unifiedEdit.isEditing, unifiedEdit.localData]);

  useEffect(() => {
    if (phrasingId && !isTransitioning) {
      const isSamePhrasing = prevPhrasingRef.current === phrasingId;
      prevPhrasingRef.current = phrasingId;

      setSelectedAnswer('');
      setHasAnsweredCurrentQuestion(false);
      clearInstantFeedback();
      setFeedbackState({
        showFeedback: false,
        nextReviewInfo: null,
      });
      setQuestionStartTime(Date.now());
      setCurrentInteractionId(null);
      setUserFeedback(null);

      if (!isSamePhrasing) {
        setOptimisticConcept(null);
        setOptimisticPhrasing(null);
      }
    }
  }, [phrasingId, isTransitioning, clearInstantFeedback]);

  const handleAnswerSelect = useCallback(
    (answer: string) => {
      if (feedbackState.showFeedback) return;
      setSelectedAnswer(answer);
    },
    [feedbackState.showFeedback]
  );

  const handleSubmit = useCallback(() => {
    if (!selectedAnswer || !question || !conceptId || !phrasingId) return;

    const submittedPhrasingId = phrasingId;
    const isCorrect = selectedAnswer === question.correctAnswer;
    setHasAnsweredCurrentQuestion(true);

    showInstantFeedback(isCorrect);
    setFeedbackState({
      showFeedback: true,
      nextReviewInfo: null,
    });

    const timeSpent = Date.now() - questionStartTime;
    trackAnswer(conceptId, submittedPhrasingId, selectedAnswer, isCorrect, timeSpent, sessionId)
      .then((reviewInfo) => {
        if (isMountedRef.current && submittedPhrasingId === phrasingId) {
          setFeedbackState({
            showFeedback: true,
            nextReviewInfo: reviewInfo
              ? {
                  nextReview: reviewInfo.nextReview,
                  scheduledDays: reviewInfo.scheduledDays,
                }
              : null,
          });
          if (reviewInfo?.interactionId) {
            setCurrentInteractionId(reviewInfo.interactionId);
          }
        }
      })
      .catch((error) => {
        console.error('Failed to track answer:', error);
        if (isMountedRef.current) {
          toast.error('Failed to save your answer', {
            description: "Your progress wasn't saved. Please try again.",
          });
        }
      });
  }, [
    selectedAnswer,
    question,
    conceptId,
    phrasingId,
    questionStartTime,
    trackAnswer,
    sessionId,
    showInstantFeedback,
  ]);

  const handleNext = useCallback(() => {
    handlers.onReviewComplete();
  }, [handlers]);

  const handleArchivePhrasing = useCallback(async () => {
    if (!phrasingId) return;

    if (totalPhrasings === 1) {
      toast.error('This is the last phrasing. Archive the entire concept instead?');
      return;
    }

    await conceptActions.archivePhrasingWithUndo(phrasingId);
    handlers.onReviewComplete();
  }, [phrasingId, totalPhrasings, conceptActions, handlers]);

  const handleArchiveConcept = useCallback(async () => {
    if (!conceptId) return;
    await conceptActions.archiveConceptWithUndo();
    handlers.onReviewComplete();
  }, [conceptId, conceptActions, handlers]);

  const [skipAnnouncement, setSkipAnnouncement] = useState('');
  const handleSkip = useCallback(() => {
    setOptimisticConcept(null);
    setOptimisticPhrasing(null);
    handlers.onSkipConcept();
    toast("Skipped. You'll see this again shortly.", { duration: 2000 });
    setSkipAnnouncement('Concept skipped. Will reappear shortly.');
    setTimeout(() => setSkipAnnouncement(''), 3000);
  }, [handlers]);

  const handleArchiveViaShortcut = useCallback(() => {
    if (!feedbackState.showFeedback) return;
    if (phrasingId && totalPhrasings !== null && totalPhrasings > 1) {
      void handleArchivePhrasing();
      return;
    }
    if (conceptId) {
      void handleArchiveConcept();
    }
  }, [
    feedbackState.showFeedback,
    phrasingId,
    totalPhrasings,
    conceptId,
    handleArchivePhrasing,
    handleArchiveConcept,
  ]);

  const handleUserFeedback = useCallback(
    async (feedbackType: 'helpful' | 'unhelpful') => {
      if (!currentInteractionId || userFeedback) return;

      setUserFeedback(feedbackType);

      try {
        await recordFeedbackMutation({
          interactionId: currentInteractionId,
          feedbackType,
        });
      } catch (error) {
        console.error('Failed to record feedback:', error);
        if (isMountedRef.current) {
          setUserFeedback(null);
          toast.error('Failed to save feedback');
        }
      }
    },
    [currentInteractionId, userFeedback, recordFeedbackMutation]
  );

  const handleStartInlineEdit = useCallback(() => {
    if (!unifiedEdit.isEditing) {
      setFeedbackState((prev) => ({ ...prev, showFeedback: true }));
      unifiedEdit.startEdit();
    }
  }, [unifiedEdit]);

  useEffect(() => {
    if (!unifiedEdit.isEditing && !hasAnsweredCurrentQuestion && feedbackState.showFeedback) {
      setFeedbackState({ showFeedback: false, nextReviewInfo: null });
    }
  }, [unifiedEdit.isEditing, hasAnsweredCurrentQuestion, feedbackState.showFeedback]);

  const handleQuestionDisplayReset = useCallback(() => {
    setSelectedAnswer('');
    setFeedbackState({ showFeedback: false, nextReviewInfo: null });
    handlers.onReviewComplete();
  }, [handlers]);

  const value: ReviewSessionContextValue = {
    phase,
    question,
    conceptTitle,
    conceptId,
    phrasingId,
    phrasingIndex,
    totalPhrasings,
    selectionReason,
    interactions,
    isTransitioning,
    conceptFsrs,
    selectedAnswer,
    feedbackState,
    hasAnsweredCurrentQuestion,
    currentInteractionId,
    userFeedback,
    displayConceptTitle,
    displayQuestion,
    phrasingPositionLabel,
    selectionReasonLabel,
    instantFeedback,
    skipAnnouncement,
    cachedDueCount,
    unifiedEdit,
    handleAnswerSelect,
    handleSubmit,
    handleNext,
    handleSkip,
    handleArchivePhrasing,
    handleArchiveConcept,
    handleUserFeedback,
    handleStartInlineEdit,
    handleArchiveViaShortcut,
    handleQuestionDisplayReset,
  };

  return <ReviewSessionContext.Provider value={value}>{children}</ReviewSessionContext.Provider>;
}

export function useReviewSession() {
  const context = useContext(ReviewSessionContext);
  if (!context) {
    throw new Error('useReviewSession must be used within ReviewSessionProvider');
  }
  return context;
}
